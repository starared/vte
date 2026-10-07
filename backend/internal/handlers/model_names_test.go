package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"vte/internal/database"
	"vte/internal/models"
)

func modelRoutes(t *testing.T, upstream http.HandlerFunc) *gin.Engine {
	t.Helper()
	setupGateway(t, upstream)
	r := gin.New()
	r.PUT("/models/:id", UpdateModel)
	r.POST("/models/:id/reset-name", ResetModelDisplayName)
	r.POST("/models/batch-toggle", BatchToggleModels)
	r.PUT("/providers/:id", UpdateProvider)
	r.POST("/providers/:id/add-model", AddModel)
	r.POST("/providers/:id/fetch-models", FetchModels)
	r.GET("/models", ListAllModels)
	r.POST("/auth/change-password", func(c *gin.Context) {
		u := &models.User{ID: 1, Username: "admin"}
		c.Set("user", u)
		ChangePassword(c)
	})
	return r
}

func addSecondProvider(t *testing.T) {
	t.Helper()
	db := database.DB()
	for _, q := range []string{
		"INSERT INTO providers(id,name,base_url,api_key,model_prefix)VALUES(2,'q','http://127.0.0.1:1','','q')",
		"INSERT INTO models(id,provider_id,original_id,display_name,is_active,source)VALUES(10,2,'other','q/other',1,'fetched')",
	} {
		if _, err := db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
}

func tempKeyModels(t *testing.T, id int) (allowed []string, limits map[string]int) {
	t.Helper()
	key, err := database.GetTempAPIKeyByID(id)
	if err != nil {
		t.Fatal(err)
	}
	return key.AllowedModels, key.ModelLimits
}

// 改模型显示名称时，临时 API 和自定义限流规则里的引用同步更新
func TestRenameUpdatesTempKeysAndRules(t *testing.T) {
	r := modelRoutes(t, func(w http.ResponseWriter, r *http.Request) {})
	key := makeTemp(t, 0, 0, 0) // allowed: p/real, limit p/real=1
	setting(t, "custom_rate_limit_rules", `[{"id":1,"name":"x","model_name":"p/real","max_requests":5,"window":60,"enabled":true}]`)

	var modelID int
	database.DB().QueryRow("SELECT id FROM models WHERE original_id='real'").Scan(&modelID)
	if rec := sendJSON(r, "PUT", fmt.Sprintf("/models/%d", modelID), "", `{"display_name":"alias"}`); rec.Code != 200 {
		t.Fatal(rec.Body.String())
	}
	allowed, limits := tempKeyModels(t, key.ID)
	if strings.Join(allowed, ",") != "alias" || limits["alias"] != 1 || len(limits) != 1 {
		t.Fatal(allowed, limits)
	}
	var rules string
	database.DB().QueryRow("SELECT value FROM settings WHERE key='custom_rate_limit_rules'").Scan(&rules)
	if !strings.Contains(rules, `"model_name":"alias"`) || !strings.Contains(rules, `"max_requests":5`) {
		t.Fatal(rules)
	}

	// 重置名称同样同步
	if rec := sendJSON(r, "POST", fmt.Sprintf("/models/%d/reset-name", modelID), "", ``); rec.Code != 200 {
		t.Fatal(rec.Body.String())
	}
	if allowed, _ := tempKeyModels(t, key.ID); strings.Join(allowed, ",") != "p/real" {
		t.Fatal(allowed)
	}
}

// 修改提供商前缀时同步引用；会造成重名时拒绝
func TestPrefixChangeSyncsAndRejectsConflicts(t *testing.T) {
	r := modelRoutes(t, func(w http.ResponseWriter, r *http.Request) {})
	key := makeTemp(t, 0, 0, 0)
	if rec := sendJSON(r, "PUT", "/providers/1", "", `{"model_prefix":"new"}`); rec.Code != 200 {
		t.Fatal(rec.Body.String())
	}
	if allowed, _ := tempKeyModels(t, key.ID); strings.Join(allowed, ",") != "new/real" {
		t.Fatal(allowed)
	}

	addSecondProvider(t)
	database.DB().Exec("INSERT INTO models(provider_id,original_id,display_name,is_active)VALUES(2,'real','q/real',1)")
	rec := sendJSON(r, "PUT", "/providers/1", "", `{"model_prefix":"q"}`)
	if rec.Code != 409 {
		t.Fatal(rec.Code, rec.Body.String())
	}
	var prefix string
	database.DB().QueryRow("SELECT model_prefix FROM providers WHERE id=1").Scan(&prefix)
	if prefix != "new" {
		t.Fatal("conflicting prefix change was partially applied:", prefix)
	}
}

func TestDuplicateDisplayNamesRejected(t *testing.T) {
	r := modelRoutes(t, func(w http.ResponseWriter, r *http.Request) {})
	addSecondProvider(t)

	// 改名成已有名称
	if rec := sendJSON(r, "PUT", "/models/10", "", `{"display_name":"p/real"}`); rec.Code != 409 {
		t.Fatal("rename to taken name", rec.Code)
	}
	// 手动添加会产生重名的模型
	database.DB().Exec("UPDATE providers SET model_prefix='' WHERE id=2")
	database.DB().Exec("UPDATE models SET display_name='other' WHERE id=10")
	database.DB().Exec("UPDATE providers SET model_prefix='' WHERE id=1")
	database.DB().Exec("UPDATE models SET display_name='real' WHERE provider_id=1")
	if rec := sendJSON(r, "POST", "/providers/2/add-model", "", `{"model_id":"real"}`); rec.Code != 409 {
		t.Fatal("duplicate manual model", rec.Code, rec.Body.String())
	}
	// 已存在的重名数据（旧版本留下的）：启用时拦截
	database.DB().Exec("INSERT INTO models(id,provider_id,original_id,display_name,is_active)VALUES(11,2,'real','real',0)")
	if rec := sendJSON(r, "PUT", "/models/11", "", `{"is_active":true}`); rec.Code != 409 {
		t.Fatal("enable duplicate", rec.Code)
	}
	if rec := sendJSON(r, "POST", "/models/batch-toggle", "", `{"model_ids":[11],"is_active":true}`); rec.Code != 409 {
		t.Fatal("batch enable duplicate", rec.Code)
	}
	// 停用不受影响
	if rec := sendJSON(r, "POST", "/models/batch-toggle", "", `{"model_ids":[11],"is_active":false}`); rec.Code != 200 {
		t.Fatal("disable", rec.Code)
	}
}

// 被同步停用的模型重新出现时自动启用；手动停用的不会
func TestFetchRestoresModelsDisabledBySync(t *testing.T) {
	list := `{"data":[{"id":"other-model"}]}`
	r := modelRoutes(t, func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, list) })
	db := database.DB()
	db.Exec("INSERT INTO models(id,provider_id,original_id,display_name,is_active,source)VALUES(20,1,'manual-off','p/manual-off',0,'')")

	sendJSON(r, "POST", "/providers/1/fetch-models", "", ``) // real 不在列表中 → 被停用
	var active, bySync int
	db.QueryRow("SELECT is_active, disabled_by_sync FROM models WHERE original_id='real'").Scan(&active, &bySync)
	if active != 0 || bySync != 1 {
		t.Fatal("not disabled by sync", active, bySync)
	}

	list = `{"data":[{"id":"real"},{"id":"manual-off"}]}`
	rec := sendJSON(r, "POST", "/providers/1/fetch-models", "", ``)
	if !strings.Contains(rec.Body.String(), "重新启用 1 个") {
		t.Fatal(rec.Body.String())
	}
	db.QueryRow("SELECT is_active, disabled_by_sync FROM models WHERE original_id='real'").Scan(&active, &bySync)
	if active != 1 || bySync != 0 {
		t.Fatal("not restored", active, bySync)
	}
	db.QueryRow("SELECT is_active FROM models WHERE original_id='manual-off'").Scan(&active)
	if active != 0 {
		t.Fatal("manually disabled model was re-enabled")
	}

	// 列表接口返回来源和同步状态
	rec = sendJSON(r, "GET", "/models", "", ``)
	var list2 []models.Model
	json.Unmarshal(rec.Body.Bytes(), &list2)
	found := false
	for _, m := range list2 {
		if m.OriginalID == "real" {
			found = m.Source == "fetched" && !m.DisabledBySync
		}
	}
	if !found {
		t.Fatal(rec.Body.String())
	}
}

func TestPasswordMinimumLength(t *testing.T) {
	r := modelRoutes(t, func(w http.ResponseWriter, r *http.Request) {})
	rec := sendJSON(r, "POST", "/auth/change-password", "", `{"old_password":"x","new_password":"short"}`)
	if rec.Code != 400 || !strings.Contains(rec.Body.String(), "8") {
		t.Fatal(rec.Code, rec.Body.String())
	}
}
