package router

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/meetmonth/dance-elite/internal/store"
)

// seedMergePlayer 插入一个选手档案并返回 ID。
func seedMergePlayer(t *testing.T, db *store.DB, playerNo string) int64 {
	t.Helper()
	res, err := db.SQL().ExecContext(context.Background(),
		`INSERT INTO players (player_no, name, id_card_hash) VALUES (?, '合并选手', ?)`,
		playerNo, "hash-"+playerNo)
	if err != nil {
		t.Fatalf("insert player %s: %v", playerNo, err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		t.Fatalf("player id: %v", err)
	}
	return id
}

// TestMergePlayerHTTP 覆盖档案合并接口的角色限制、响应字段与错误码。
func TestMergePlayerHTTP(t *testing.T) {
	engine, signer, db := newRefundTestEngine(t)
	adminHeaders := map[string]string{"Authorization": "Bearer " + adminToken(t, engine)}
	_, csHeaders := seedCSAdmin(t, db, signer)
	sourceID := seedMergePlayer(t, db, "MergeS01")
	targetID := seedMergePlayer(t, db, "MergeT01")
	path := fmt.Sprintf("/api/v1/admin/players/%d/merge", sourceID)
	body := fmt.Sprintf(`{"target_player_id": %d}`, targetID)

	// 未登录 → 40101；cs → 40301。
	assertCode(t, postJSON(t, engine, http.MethodPost, path, body, nil), 40101)
	assertCode(t, postJSON(t, engine, http.MethodPost, path, body, csHeaders), 40301)

	// 参数缺失 → 40001；自身合并 → 40001。
	assertCode(t, postJSON(t, engine, http.MethodPost, path, `{}`, adminHeaders), 40001)
	assertCode(t, postJSON(t, engine, http.MethodPost,
		fmt.Sprintf("/api/v1/admin/players/%d/merge", sourceID),
		fmt.Sprintf(`{"target_player_id": %d}`, sourceID), adminHeaders), 40001)

	// 成功：响应字段收敛。
	w := postJSON(t, engine, http.MethodPost, path, body, adminHeaders)
	if w.Code != http.StatusOK {
		t.Fatalf("merge status = %d, body = %s", w.Code, w.Body)
	}
	var resp struct {
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode merge: %v", err)
	}
	assertSnakeCaseKeys(t, "merge player", resp.Data)
	assertObjectKeys(t, "merge player", resp.Data, "player_id", "merged_into", "moved")
	if resp.Data["merged_into"] != float64(targetID) {
		t.Fatalf("merged_into = %v, want %d", resp.Data["merged_into"], targetID)
	}

	// 重复合并 → 40901；选手不存在 → 40401。
	assertCode(t, postJSON(t, engine, http.MethodPost, path, body, adminHeaders), 40901)
	assertCode(t, postJSON(t, engine, http.MethodPost,
		"/api/v1/admin/players/999999/merge", body, adminHeaders), 40401)
}
