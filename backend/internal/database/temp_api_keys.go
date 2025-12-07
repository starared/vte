package database

import (
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"vte/internal/models"
)

var (
	ErrTempAPIExpired       = errors.New("temp api key expired")
	ErrTempAPIDisabled      = errors.New("temp api key disabled")
	ErrTempAPILimitExceeded = errors.New("temp api key request limit exceeded")
	ErrTempAPIModelExceeded = errors.New("temp api key model quota exceeded")
)

func parseDBTime(raw string) *time.Time {
	if raw == "" {
		return nil
	}
	layouts := []string{time.RFC3339, "2006-01-02 15:04:05", time.RFC3339Nano, "2006-01-02T15:04:05Z"}
	for _, layout := range layouts {
		if t, err := time.Parse(layout, raw); err == nil {
			return &t
		}
	}
	return nil
}

func ensureUsageMap(models []string, usage map[string]int) map[string]int {
	if usage == nil {
		usage = make(map[string]int)
	}
	for _, m := range models {
		if _, ok := usage[m]; !ok {
			usage[m] = 0
		}
	}
	return usage
}

// calculateExpiresAt 根据时长和单位计算过期时间
func calculateExpiresAt(duration int, unit string) *time.Time {
	if duration <= 0 {
		return nil
	}
	var d time.Duration
	switch unit {
	case "minutes":
		d = time.Duration(duration) * time.Minute
	case "hours":
		d = time.Duration(duration) * time.Hour
	case "days":
		d = time.Duration(duration) * 24 * time.Hour
	default:
		return nil
	}
	t := time.Now().Add(d)
	return &t
}

func scanTempAPIKey(row *sql.Row) (*models.TempAPIKey, error) {
	var key models.TempAPIKey
	var allowedStr, limitsStr, usageStr, expiresRaw, expireUnit, rateLimitUnit sql.NullString
	var isActive int

	err := row.Scan(
		&key.ID,
		&key.Name,
		&key.Token,
		&allowedStr,
		&limitsStr,
		&usageStr,
		&key.MaxRequests,
		&key.UsedRequests,
		&key.RateLimitCount,
		&key.RateLimitWindow,
		&rateLimitUnit,
		&key.ConcurrencyLimit,
		&key.ExpireDuration,
		&expireUnit,
		&expiresRaw,
		&isActive,
		&key.CreatedAt,
		&key.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}

	key.IsActive = isActive == 1

	if allowedStr.Valid && allowedStr.String != "" {
		json.Unmarshal([]byte(allowedStr.String), &key.AllowedModels)
	}
	if limitsStr.Valid && limitsStr.String != "" {
		json.Unmarshal([]byte(limitsStr.String), &key.ModelLimits)
	}
	if usageStr.Valid && usageStr.String != "" {
		json.Unmarshal([]byte(usageStr.String), &key.ModelUsage)
	}
	key.ModelUsage = ensureUsageMap(key.AllowedModels, key.ModelUsage)

	if rateLimitUnit.Valid {
		key.RateLimitUnit = rateLimitUnit.String
	}
	if expireUnit.Valid {
		key.ExpireUnit = expireUnit.String
	}
	if expiresRaw.Valid && expiresRaw.String != "" {
		key.ExpiresAt = parseDBTime(expiresRaw.String)
	}

	return &key, nil
}

// CreateTempAPIKey 创建临时 API Key
func CreateTempAPIKey(req models.TempAPIKeyCreateRequest) (*models.TempAPIKey, error) {
	token := generateAPIKey()

	allowedJSON, _ := json.Marshal(req.AllowedModels)
	limitsJSON, _ := json.Marshal(req.ModelLimits)
	usageJSON, _ := json.Marshal(ensureUsageMap(req.AllowedModels, nil))

	// 计算过期时间
	var expiresAt interface{}
	if req.ExpireDuration > 0 && req.ExpireUnit != "" {
		if t := calculateExpiresAt(req.ExpireDuration, req.ExpireUnit); t != nil {
			expiresAt = t.Format(time.RFC3339)
		}
	}

	isActive := 1
	if req.IsActive != nil && !*req.IsActive {
		isActive = 0
	}

	res, err := db.Exec(`
        INSERT INTO temp_api_keys (name, token, allowed_models, model_limits, model_usage, max_requests, used_requests, rate_limit_count, rate_limit_window, rate_limit_unit, concurrency_limit, expire_duration, expire_unit, expires_at, is_active)
        VALUES (?, ?, ?, ?, ?, ?, 0, ?, ?, ?, ?, ?, ?, ?, ?)
    `, req.Name, token, string(allowedJSON), string(limitsJSON), string(usageJSON), req.MaxRequests, req.RateLimitCount, req.RateLimitWindow, req.RateLimitUnit, req.ConcurrencyLimit, req.ExpireDuration, req.ExpireUnit, expiresAt, isActive)
	if err != nil {
		return nil, err
	}

	id, _ := res.LastInsertId()
	return GetTempAPIKeyByID(int(id))
}

// GetTempAPIKeyByID 获取单个临时密钥
func GetTempAPIKeyByID(id int) (*models.TempAPIKey, error) {
	row := db.QueryRow(`
        SELECT id, name, token, allowed_models, model_limits, model_usage, max_requests, used_requests, 
               COALESCE(rate_limit_count, 0), COALESCE(rate_limit_window, 0), COALESCE(rate_limit_unit, ''),
               COALESCE(concurrency_limit, 0),
               COALESCE(expire_duration, 0), COALESCE(expire_unit, ''), COALESCE(expires_at, ''), 
               is_active, created_at, updated_at
        FROM temp_api_keys
        WHERE id = ?
    `, id)
	return scanTempAPIKey(row)
}

// GetTempAPIKeyByToken 根据 token 获取临时密钥
func GetTempAPIKeyByToken(token string) (*models.TempAPIKey, error) {
	row := db.QueryRow(`
        SELECT id, name, token, allowed_models, model_limits, model_usage, max_requests, used_requests, 
               COALESCE(rate_limit_count, 0), COALESCE(rate_limit_window, 0), COALESCE(rate_limit_unit, ''),
               COALESCE(concurrency_limit, 0),
               COALESCE(expire_duration, 0), COALESCE(expire_unit, ''), COALESCE(expires_at, ''), 
               is_active, created_at, updated_at
        FROM temp_api_keys
        WHERE token = ?
    `, token)
	return scanTempAPIKey(row)
}

// ListTempAPIKeys 列出所有临时密钥
func ListTempAPIKeys() ([]*models.TempAPIKey, error) {
	rows, err := db.Query(`
        SELECT id, name, token, allowed_models, model_limits, model_usage, max_requests, used_requests, 
               COALESCE(rate_limit_count, 0), COALESCE(rate_limit_window, 0), COALESCE(rate_limit_unit, ''),
               COALESCE(concurrency_limit, 0),
               COALESCE(expire_duration, 0), COALESCE(expire_unit, ''), COALESCE(expires_at, ''), 
               is_active, created_at, updated_at
        FROM temp_api_keys
        ORDER BY created_at DESC
    `)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []*models.TempAPIKey
	for rows.Next() {
		var key models.TempAPIKey
		var allowedStr, limitsStr, usageStr, expiresRaw, expireUnit, rateLimitUnit sql.NullString
		var isActive int

		if err := rows.Scan(
			&key.ID,
			&key.Name,
			&key.Token,
			&allowedStr,
			&limitsStr,
			&usageStr,
			&key.MaxRequests,
			&key.UsedRequests,
			&key.RateLimitCount,
			&key.RateLimitWindow,
			&rateLimitUnit,
			&key.ConcurrencyLimit,
			&key.ExpireDuration,
			&expireUnit,
			&expiresRaw,
			&isActive,
			&key.CreatedAt,
			&key.UpdatedAt,
		); err != nil {
			return nil, err
		}

		key.IsActive = isActive == 1
		if allowedStr.Valid && allowedStr.String != "" {
			json.Unmarshal([]byte(allowedStr.String), &key.AllowedModels)
		}
		if limitsStr.Valid && limitsStr.String != "" {
			json.Unmarshal([]byte(limitsStr.String), &key.ModelLimits)
		}
		if usageStr.Valid && usageStr.String != "" {
			json.Unmarshal([]byte(usageStr.String), &key.ModelUsage)
		}
		key.ModelUsage = ensureUsageMap(key.AllowedModels, key.ModelUsage)

		if rateLimitUnit.Valid {
			key.RateLimitUnit = rateLimitUnit.String
		}
		if expireUnit.Valid {
			key.ExpireUnit = expireUnit.String
		}
		if expiresRaw.Valid && expiresRaw.String != "" {
			key.ExpiresAt = parseDBTime(expiresRaw.String)
		}

		list = append(list, &key)
	}
	return list, nil
}

// UpdateTempAPIKey 更新临时密钥
func UpdateTempAPIKey(id int, req models.TempAPIKeyUpdateRequest) (*models.TempAPIKey, error) {
	key, err := GetTempAPIKeyByID(id)
	if err != nil {
		return nil, err
	}

	if req.Name != nil {
		key.Name = *req.Name
	}
	if len(req.AllowedModels) > 0 {
		key.AllowedModels = req.AllowedModels
	}
	if req.ModelLimits != nil {
		key.ModelLimits = req.ModelLimits
	}
	if req.MaxRequests != nil {
		key.MaxRequests = *req.MaxRequests
	}
	if req.RateLimitCount != nil {
		key.RateLimitCount = *req.RateLimitCount
	}
	if req.RateLimitWindow != nil {
		key.RateLimitWindow = *req.RateLimitWindow
	}
	if req.RateLimitUnit != nil {
		key.RateLimitUnit = *req.RateLimitUnit
	}
	if req.ConcurrencyLimit != nil {
		key.ConcurrencyLimit = *req.ConcurrencyLimit
	}
	if req.ExpireDuration != nil {
		key.ExpireDuration = *req.ExpireDuration
	}
	if req.ExpireUnit != nil {
		key.ExpireUnit = *req.ExpireUnit
	}
	// 如果更新了有效期设置，重新计算过期时间
	if req.ExpireDuration != nil || req.ExpireUnit != nil {
		key.ExpiresAt = calculateExpiresAt(key.ExpireDuration, key.ExpireUnit)
	}
	if req.IsActive != nil {
		key.IsActive = *req.IsActive
	}

	key.ModelUsage = ensureUsageMap(key.AllowedModels, key.ModelUsage)

	allowedJSON, _ := json.Marshal(key.AllowedModels)
	limitsJSON, _ := json.Marshal(key.ModelLimits)
	usageJSON, _ := json.Marshal(key.ModelUsage)

	var expiresAt interface{}
	if key.ExpiresAt != nil {
		expiresAt = key.ExpiresAt.Format(time.RFC3339)
	}

	isActive := 0
	if key.IsActive {
		isActive = 1
	}

	_, err = db.Exec(`
        UPDATE temp_api_keys
        SET name = ?, allowed_models = ?, model_limits = ?, model_usage = ?, max_requests = ?, used_requests = ?, 
            rate_limit_count = ?, rate_limit_window = ?, rate_limit_unit = ?, concurrency_limit = ?,
            expire_duration = ?, expire_unit = ?, expires_at = ?, 
            is_active = ?, updated_at = CURRENT_TIMESTAMP
        WHERE id = ?
    `, key.Name, string(allowedJSON), string(limitsJSON), string(usageJSON), key.MaxRequests, key.UsedRequests,
		key.RateLimitCount, key.RateLimitWindow, key.RateLimitUnit, key.ConcurrencyLimit,
		key.ExpireDuration, key.ExpireUnit, expiresAt,
		isActive, key.ID)
	if err != nil {
		return nil, err
	}

	return key, nil
}

// DeleteTempAPIKey 删除
func DeleteTempAPIKey(id int) error {
	_, err := db.Exec("DELETE FROM temp_api_keys WHERE id = ?", id)
	return err
}

// ConsumeTempAPIUsage 消耗一次请求并更新计数
func ConsumeTempAPIUsage(keyID int, model string) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			tx.Rollback()
		}
	}()

	var maxReq, usedReq int
	var limitsStr, usageStr, expiresRaw sql.NullString
	var isActive int

	row := tx.QueryRow(`
        SELECT max_requests, used_requests, model_limits, model_usage, COALESCE(expires_at, ''), is_active
        FROM temp_api_keys WHERE id = ?
    `, keyID)
	if err = row.Scan(&maxReq, &usedReq, &limitsStr, &usageStr, &expiresRaw, &isActive); err != nil {
		return err
	}

	if isActive == 0 {
		return ErrTempAPIDisabled
	}
	if expiresRaw.Valid && expiresRaw.String != "" {
		if t := parseDBTime(expiresRaw.String); t != nil && time.Now().After(*t) {
			return ErrTempAPIExpired
		}
	}
	if maxReq > 0 && usedReq >= maxReq {
		return ErrTempAPILimitExceeded
	}

	limits := map[string]int{}
	usage := map[string]int{}
	if limitsStr.Valid && limitsStr.String != "" {
		json.Unmarshal([]byte(limitsStr.String), &limits)
	}
	if usageStr.Valid && usageStr.String != "" {
		json.Unmarshal([]byte(usageStr.String), &usage)
	}

	if limit, ok := limits[model]; ok {
		if limit > 0 && usage[model] >= limit {
			return ErrTempAPIModelExceeded
		}
	}

	usage[model]++
	usedReq++

	usageJSON, _ := json.Marshal(usage)

	_, err = tx.Exec(`
        UPDATE temp_api_keys
        SET used_requests = ?, model_usage = ?, updated_at = CURRENT_TIMESTAMP
        WHERE id = ?
    `, usedReq, string(usageJSON), keyID)
	if err != nil {
		return err
	}

	return tx.Commit()
}
