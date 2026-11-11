package router

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/meetmonth/dance-elite/internal/model"
	"github.com/meetmonth/dance-elite/internal/pkg/token"
	"github.com/meetmonth/dance-elite/internal/store"
)

// seedCSAdmin 插入一个独立客服账号并签发 token（列表角色隔离测试用）。
func seedCSAdmin(t *testing.T, db *store.DB, signer *token.Signer) (int64, map[string]string) {
	t.Helper()
	res, err := db.SQL().ExecContext(context.Background(),
		`INSERT INTO admins (username, password_hash, role) VALUES ('cs-http', 'x', 'cs')`)
	if err != nil {
		t.Fatalf("insert cs admin: %v", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		t.Fatalf("cs admin id: %v", err)
	}
	raw, err := signer.IssueAdmin(id, model.RoleCS)
	if err != nil {
		t.Fatalf("issue cs token: %v", err)
	}
	return id, map[string]string{"Authorization": "Bearer " + raw}
}

// createCSTicketHTTP 通过接口登记工单，返回工单 ID。
func createCSTicketHTTP(t *testing.T, engine *gin.Engine, headers map[string]string, body string) int64 {
	t.Helper()
	w := postJSON(t, engine, http.MethodPost, "/api/v1/admin/cs-tickets", body, headers)
	if w.Code != http.StatusOK {
		t.Fatalf("create cs ticket status = %d, body = %s", w.Code, w.Body)
	}
	var created struct {
		Data struct {
			TicketID int64 `json:"ticket_id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode cs ticket: %v", err)
	}
	return created.Data.TicketID
}

// TestCSTicketCreateListHTTP 覆盖登记/列表接口的鉴权、字段收敛、角色隔离与错误码。
func TestCSTicketCreateListHTTP(t *testing.T) {
	engine, signer, db := newRefundTestEngine(t)
	adminHeaders := map[string]string{"Authorization": "Bearer " + adminToken(t, engine)}
	_, csHeaders := seedCSAdmin(t, db, signer)
	path := "/api/v1/admin/cs-tickets"

	// 未登录 → 40101。
	assertCode(t, postJSON(t, engine, http.MethodPost, path,
		`{"type":"other","phone":"138","content":"咨询"}`, nil), 40101)

	// cs 登记 → 200，响应字段收敛。
	w := postJSON(t, engine, http.MethodPost, path,
		`{"type":"no_id_card","user_name":"Emma","phone":"13800000001","content":"外籍选手报名咨询"}`, csHeaders)
	if w.Code != http.StatusOK {
		t.Fatalf("cs create status = %d, body = %s", w.Code, w.Body)
	}
	var created struct {
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode create: %v", err)
	}
	assertSnakeCaseKeys(t, "create cs ticket", created.Data)
	assertObjectKeys(t, "create cs ticket", created.Data, "ticket_id", "status")
	if created.Data["status"] != "pending" {
		t.Fatalf("created status = %v, want pending", created.Data["status"])
	}

	// admin 也登记一条退款工单。
	createCSTicketHTTP(t, engine, adminHeaders,
		`{"type":"refund","phone":"13800000002","content":"退款咨询"}`)

	// cs 列表只看自己登记的。
	var csList struct {
		Data struct {
			Total int64 `json:"total"`
		} `json:"data"`
	}
	if err := json.Unmarshal(postJSON(t, engine, http.MethodGet, path, "", csHeaders).Body.Bytes(), &csList); err != nil {
		t.Fatalf("decode cs list: %v", err)
	}
	if csList.Data.Total != 1 {
		t.Fatalf("cs list total = %d, want 1", csList.Data.Total)
	}

	// admin 列表看全量。
	w = postJSON(t, engine, http.MethodGet, path, "", adminHeaders)
	var adminList struct {
		Data struct {
			Total int64            `json:"total"`
			List  []map[string]any `json:"list"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &adminList); err != nil {
		t.Fatalf("decode admin list: %v", err)
	}
	if adminList.Data.Total != 2 || len(adminList.Data.List) != 2 {
		t.Fatalf("admin list = %+v, want 2", adminList.Data)
	}
	assertSnakeCaseKeys(t, "cs ticket list item", adminList.Data.List[0])
	assertObjectKeys(t, "cs ticket list item", adminList.Data.List[0],
		"ticket_id", "type", "status", "created_by", "created_at")

	// 参数错误 → 40001；关联订单不存在 → 40401。
	assertCode(t, postJSON(t, engine, http.MethodPost, path,
		`{"type":"bogus","phone":"138","content":"内容"}`, adminHeaders), 40001)
	assertCode(t, postJSON(t, engine, http.MethodPost, path,
		`{"type":"other","phone":"138","content":"内容","related_order_no":"ORD-missing"}`, adminHeaders), 40401)
	assertCode(t, postJSON(t, engine, http.MethodGet, path+"?status=bogus", "", adminHeaders), 40001)
}

// TestCSTicketUpdateHTTP 覆盖流转接口的角色限制、状态机与错误码。
func TestCSTicketUpdateHTTP(t *testing.T) {
	engine, signer, db := newRefundTestEngine(t)
	adminHeaders := map[string]string{"Authorization": "Bearer " + adminToken(t, engine)}
	_, csHeaders := seedCSAdmin(t, db, signer)
	ticketID := createCSTicketHTTP(t, engine, adminHeaders,
		`{"type":"other","phone":"13800000003","content":"咨询比赛时间"}`)

	// cs 无流转权限 → 40301。
	assertCode(t, postJSON(t, engine, http.MethodPut,
		fmt.Sprintf("/api/v1/admin/cs-tickets/%d", ticketID), `{"status":"processing"}`, csHeaders), 40301)

	// 参数缺失 → 40001。
	assertCode(t, postJSON(t, engine, http.MethodPut,
		fmt.Sprintf("/api/v1/admin/cs-tickets/%d", ticketID), `{}`, adminHeaders), 40001)

	// 流转成功。
	w := postJSON(t, engine, http.MethodPut,
		fmt.Sprintf("/api/v1/admin/cs-tickets/%d", ticketID),
		`{"status":"processing","remark":"已联系用户"}`, adminHeaders)
	if w.Code != http.StatusOK {
		t.Fatalf("update status = %d, body = %s", w.Code, w.Body)
	}
	var resp struct {
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode update: %v", err)
	}
	assertObjectKeys(t, "update cs ticket", resp.Data, "ticket_id", "status")
	if resp.Data["status"] != "processing" {
		t.Fatalf("update status = %v", resp.Data["status"])
	}

	// 回退流转 → 41013；工单不存在 → 40401。
	assertCode(t, postJSON(t, engine, http.MethodPut,
		fmt.Sprintf("/api/v1/admin/cs-tickets/%d", ticketID), `{"status":"pending"}`, adminHeaders), 41013)
	assertCode(t, postJSON(t, engine, http.MethodPut,
		"/api/v1/admin/cs-tickets/999999", `{"status":"processing"}`, adminHeaders), 40401)
}
