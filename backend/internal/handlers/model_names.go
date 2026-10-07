package handlers

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
)

// queryer 同时适用于 *sql.DB 和 *sql.Tx
type queryer interface {
	Exec(query string, args ...interface{}) (sql.Result, error)
	Query(query string, args ...interface{}) (*sql.Rows, error)
	QueryRow(query string, args ...interface{}) *sql.Row
}

// placeholders 生成 "?,?,?"
func placeholders(n int) string {
	return strings.TrimSuffix(strings.Repeat("?,", n), ",")
}

func intArgs(ids []int) []interface{} {
	args := make([]interface{}, len(ids))
	for i, id := range ids {
		args[i] = id
	}
	return args
}

// displayNameTaken 检查是否有其他模型（不在 excludeIDs 中）已经使用了这个显示名称。
// 显示名称是客户端调用时使用的模型名，重名会导致请求报「模型名称有歧义」。
func displayNameTaken(q queryer, name string, excludeIDs ...int) (bool, error) {
	query := "SELECT COUNT(*) FROM models WHERE COALESCE(display_name, original_id) = ?"
	args := []interface{}{name}
	if len(excludeIDs) > 0 {
		query += " AND id NOT IN (" + placeholders(len(excludeIDs)) + ")"
		args = append(args, intArgs(excludeIDs)...)
	}
	var n int
	if err := q.QueryRow(query, args...).Scan(&n); err != nil {
		return false, err
	}
	return n > 0, nil
}

// activeNameConflicts 返回 ids 中这些模型启用后会与其他已启用模型重名的显示名称
// （包括 ids 内部互相重名的情况）
func activeNameConflicts(q queryer, ids []int) ([]string, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	rows, err := q.Query("SELECT COALESCE(display_name, original_id) FROM models WHERE id IN ("+placeholders(len(ids))+")", intArgs(ids)...)
	if err != nil {
		return nil, err
	}
	seen := map[string]int{}
	var names []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			rows.Close()
			return nil, err
		}
		if seen[name] == 0 {
			names = append(names, name)
		}
		seen[name]++
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	var conflicts []string
	for _, name := range names {
		if seen[name] > 1 {
			conflicts = append(conflicts, name)
			continue
		}
		args := append([]interface{}{name}, intArgs(ids)...)
		var n int
		if err := q.QueryRow(`SELECT COUNT(*) FROM models WHERE is_active = 1
			AND COALESCE(display_name, original_id) = ? AND id NOT IN (`+placeholders(len(ids))+`)`, args...).Scan(&n); err != nil {
			return nil, err
		}
		if n > 0 {
			conflicts = append(conflicts, name)
		}
	}
	return conflicts, nil
}

func duplicateNameError(names []string) error {
	return fmt.Errorf("模型名称与其他已启用模型重复：%s。请先修改显示名称或给提供商设置前缀", strings.Join(names, "、"))
}

// renameModelReferences 模型显示名称变化后，同步更新引用了旧名称的配置：
// 临时 API 的可用模型、各模型次数限制与已用次数，以及自定义速率限制规则。
// 必须在修改模型名称的同一个事务里调用。
func renameModelReferences(q queryer, renames map[string]string) error {
	for oldName, newName := range renames {
		if oldName == newName {
			delete(renames, oldName)
		}
	}
	if len(renames) == 0 {
		return nil
	}

	// 临时 API 密钥
	type tempRow struct {
		id                    int
		allowed, limits, used sql.NullString
	}
	rows, err := q.Query("SELECT id, allowed_models, model_limits, model_usage FROM temp_api_keys")
	if err != nil {
		return err
	}
	var temps []tempRow
	for rows.Next() {
		var r tempRow
		if err := rows.Scan(&r.id, &r.allowed, &r.limits, &r.used); err != nil {
			rows.Close()
			return err
		}
		temps = append(temps, r)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}

	for _, r := range temps {
		allowed, changedA, err := renameInList(r.allowed.String, renames)
		if err != nil {
			continue // 格式异常的旧数据保持原样
		}
		limits, changedL, err := renameInMap(r.limits.String, renames)
		if err != nil {
			continue
		}
		used, changedU, err := renameInMap(r.used.String, renames)
		if err != nil {
			continue
		}
		if !changedA && !changedL && !changedU {
			continue
		}
		if _, err := q.Exec("UPDATE temp_api_keys SET allowed_models = ?, model_limits = ?, model_usage = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?",
			allowed, limits, used, r.id); err != nil {
			return err
		}
	}

	// 自定义速率限制规则（保留未知字段，只改 model_name）
	var rulesJSON string
	err = q.QueryRow("SELECT value FROM settings WHERE key = 'custom_rate_limit_rules'").Scan(&rulesJSON)
	if err == sql.ErrNoRows || rulesJSON == "" {
		return nil
	}
	if err != nil {
		return err
	}
	var rules []map[string]interface{}
	if json.Unmarshal([]byte(rulesJSON), &rules) != nil {
		return nil
	}
	changed := false
	for _, rule := range rules {
		if name, ok := rule["model_name"].(string); ok {
			if newName, ok := renames[name]; ok {
				rule["model_name"] = newName
				changed = true
			}
		}
	}
	if !changed {
		return nil
	}
	encoded, err := json.Marshal(rules)
	if err != nil {
		return err
	}
	_, err = q.Exec("UPDATE settings SET value = ? WHERE key = 'custom_rate_limit_rules'", string(encoded))
	return err
}

func renameInList(raw string, renames map[string]string) (string, bool, error) {
	if raw == "" {
		return raw, false, nil
	}
	var list []string
	if err := json.Unmarshal([]byte(raw), &list); err != nil {
		return raw, false, err
	}
	changed := false
	for i, name := range list {
		if newName, ok := renames[name]; ok {
			list[i] = newName
			changed = true
		}
	}
	if !changed {
		return raw, false, nil
	}
	out, err := json.Marshal(list)
	return string(out), true, err
}

func renameInMap(raw string, renames map[string]string) (string, bool, error) {
	if raw == "" {
		return raw, false, nil
	}
	var m map[string]int
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		return raw, false, err
	}
	changed := false
	for oldName, newName := range renames {
		if v, ok := m[oldName]; ok {
			delete(m, oldName)
			m[newName] += v
			changed = true
		}
	}
	if !changed {
		return raw, false, nil
	}
	out, err := json.Marshal(m)
	return string(out), true, err
}
