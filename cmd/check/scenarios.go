package main

import (
	"fmt"
	"strings"
	"sync"
)

const (
	demoApiaryID = "apiary-north"
	demoHiveID   = "hive-01"
)

func seedApiary(c *client, id, name, district, keeper string) error {
	_, err := c.expect("POST", "/apiaries", map[string]any{
		"id":       id,
		"name":     name,
		"district": district,
		"keeper":   keeper,
	}, 201, `"id":"`+id+`"`)
	return err
}

func seedHive(c *client, apiaryID, hiveID, model string) error {
	_, err := c.expect("POST", "/apiaries/"+apiaryID+"/hives", map[string]any{
		"id":    hiveID,
		"model": model,
	}, 201, `"state":"colonized"`)
	return err
}

func seedDefault(c *client) error {
	if err := seedApiary(c, demoApiaryID, "北岭蜂场", "北岭片区", "周岚"); err != nil {
		return err
	}
	return seedHive(c, demoApiaryID, demoHiveID, "langstroth")
}

func checkRegisterApiary(c *client) error {
	if _, err := c.expect("POST", "/apiaries", map[string]any{
		"id": "apiary-north", "name": "北岭蜂场", "district": "北岭片区", "keeper": "周岚",
	}, 201, `"id":"apiary-north"`, `"name":"北岭蜂场"`); err != nil {
		return err
	}
	if _, err := c.expect("GET", "/apiaries/apiary-north", nil, 200,
		`"district":"北岭片区"`, `"keeper":"周岚"`); err != nil {
		return err
	}
	if _, err := c.expect("POST", "/apiaries", map[string]any{
		"id": "apiary-north", "name": "重复蜂场", "district": "北岭片区", "keeper": "周岚",
	}, 409, `"code":"conflict"`); err != nil {
		return err
	}
	if _, err := c.expect("POST", "/apiaries", map[string]any{
		"id": "apiary-south", "name": "", "district": "南坡片区", "keeper": "李澈",
	}, 400, `"code":"invalid_request"`); err != nil {
		return err
	}
	return nil
}

func checkHiveInstall(c *client) error {
	if err := seedApiary(c, demoApiaryID, "北岭蜂场", "北岭片区", "周岚"); err != nil {
		return err
	}
	if _, err := c.expect("POST", "/apiaries/apiary-north/hives", map[string]any{
		"id": "hive-01", "model": "langstroth",
	}, 201, `"state":"colonized"`, `"apiaryId":"apiary-north"`); err != nil {
		return err
	}
	if _, err := c.expect("GET", "/hives/hive-01", nil, 200, `"model":"langstroth"`); err != nil {
		return err
	}
	if _, err := c.expect("POST", "/apiaries/apiary-north/hives", map[string]any{
		"id": "hive-01", "model": "warre",
	}, 409, `"code":"conflict"`); err != nil {
		return err
	}
	if _, err := c.expect("POST", "/apiaries/apiary-missing/hives", map[string]any{
		"id": "hive-09", "model": "warre",
	}, 404, `"code":"not_found"`); err != nil {
		return err
	}
	if _, err := c.expect("POST", "/apiaries/apiary-north/hives", map[string]any{
		"id": "hive-02", "model": "dadant",
	}, 400, `"code":"invalid_request"`); err != nil {
		return err
	}
	return nil
}

func checkRecordInspection(c *client) error {
	if err := seedDefault(c); err != nil {
		return err
	}
	if _, err := c.expect("POST", "/hives/hive-01/inspections", map[string]any{
		"observedAt": "2026-04-10T09:00:00Z", "colonyStrength": 9, "broodPattern": "patchy",
		"temper": "calm", "miteCount": 3, "note": "",
	}, 400, `"code":"invalid_request"`); err != nil {
		return err
	}
	if _, err := c.expect("POST", "/hives/hive-01/inspections", map[string]any{
		"observedAt": "2026-04-10T09:00:00Z", "colonyStrength": 3, "broodPattern": "patchy",
		"temper": "aggressive", "miteCount": 12, "note": "西侧巢门观察",
	}, 201, `"inspection-000001"`, `"mite_load"`, `"temper_risk"`); err != nil {
		return err
	}
	if _, err := c.expect("GET", "/hives/hive-01/inspections", nil, 200, `"note":"西侧巢门观察"`); err != nil {
		return err
	}
	if _, err := c.expect("POST", "/hives/hive-01/state", map[string]any{"target": "retired"}, 200,
		`"to":"retired"`); err != nil {
		return err
	}
	if _, err := c.expect("POST", "/hives/hive-01/inspections", map[string]any{
		"observedAt": "2026-04-11T09:00:00Z", "colonyStrength": 3, "broodPattern": "solid",
		"temper": "calm", "miteCount": 1, "note": "",
	}, 409, `"code":"invalid_state"`); err != nil {
		return err
	}
	return nil
}

func checkReplaceQueen(c *client) error {
	if err := seedDefault(c); err != nil {
		return err
	}
	if _, err := c.expect("POST", "/hives/hive-01/queens", map[string]any{
		"origin": "bred", "introducedAt": "2026-04-02T08:00:00Z", "reason": "aging",
	}, 201, `"queen-000001"`, `"markColor":"white"`, `"state":"laying"`); err != nil {
		return err
	}
	if _, err := c.expect("POST", "/hives/hive-01/queens", map[string]any{
		"origin": "cells", "introducedAt": "2026-04-20T08:00:00Z", "reason": "aggression",
	}, 201, `"queen-000002"`, `"markColor":"white"`); err != nil {
		return err
	}
	if _, err := c.expect("GET", "/hives/hive-01/queens", nil, 200,
		`"queen-000001"`, `"superseded"`, `"queen-000002"`); err != nil {
		return err
	}
	if _, err := c.expect("POST", "/hives/hive-01/queens", map[string]any{
		"origin": "imported", "introducedAt": "2026-05-01T08:00:00Z", "reason": "aging",
	}, 400, `"code":"invalid_request"`); err != nil {
		return err
	}
	if _, err := c.expect("POST", "/hives/hive-01/state", map[string]any{"target": "retired"}, 200,
		`"to":"retired"`); err != nil {
		return err
	}
	if _, err := c.expect("POST", "/hives/hive-01/queens", map[string]any{
		"origin": "bred", "introducedAt": "2026-05-02T08:00:00Z", "reason": "loss",
	}, 409, `"code":"invalid_state"`); err != nil {
		return err
	}
	return nil
}

func checkAdvanceHiveState(c *client) error {
	if err := seedDefault(c); err != nil {
		return err
	}
	if _, err := c.expect("POST", "/hives/hive-01/state", map[string]any{"target": "production"}, 200,
		`"from":"colonized"`, `"to":"production"`); err != nil {
		return err
	}
	if _, err := c.expect("POST", "/hives/hive-01/state", map[string]any{"target": "colonized"}, 409,
		`"code":"invalid_state"`); err != nil {
		return err
	}
	if _, err := c.expect("POST", "/hives/hive-01/state", map[string]any{"target": "production"}, 409,
		`"code":"invalid_state"`); err != nil {
		return err
	}
	if _, err := c.expect("POST", "/hives/hive-01/state", map[string]any{"target": "retired"}, 200,
		`"to":"retired"`); err != nil {
		return err
	}
	if _, err := c.expect("POST", "/hives/hive-01/state", map[string]any{"target": "production"}, 409,
		`"code":"invalid_state"`); err != nil {
		return err
	}
	if _, err := c.expect("POST", "/hives/hive-missing/state", map[string]any{"target": "production"}, 404,
		`"code":"not_found"`); err != nil {
		return err
	}
	return nil
}

func checkReadApiaryStatus(c *client) error {
	if err := seedApiary(c, demoApiaryID, "北岭蜂场", "北岭片区", "周岚"); err != nil {
		return err
	}
	for _, hiveID := range []string{"hive-01", "hive-02", "hive-03"} {
		if err := seedHive(c, demoApiaryID, hiveID, "langstroth"); err != nil {
			return err
		}
	}
	if _, err := c.expect("POST", "/hives/hive-02/state", map[string]any{"target": "production"}, 200,
		`"to":"production"`); err != nil {
		return err
	}
	if _, err := c.expect("POST", "/hives/hive-01/inspections", map[string]any{
		"observedAt": "2026-04-10T09:00:00Z", "colonyStrength": 4, "broodPattern": "solid",
		"temper": "calm", "miteCount": 12, "note": "",
	}, 201, `"mite_load"`); err != nil {
		return err
	}
	report, err := c.expect("GET", "/apiaries/apiary-north/status?asOf=2026-04-12T00:00:00Z", nil, 200,
		`"hiveCount":3`, `"colonized":2`, `"production":1`, `"layingQueens":0`)
	if err != nil {
		return err
	}
	for _, fragment := range []string{
		`"needsInspection":["hive-02","hive-03"]`,
		`"concernHives":["hive-01"]`,
	} {
		if !strings.Contains(report.text(), fragment) {
			return fmt.Errorf("状态汇总缺少 %q：%s", fragment, report.text())
		}
	}
	if err := report.mustOrder(`"hiveId":"hive-01"`, `"hiveId":"hive-02"`); err != nil {
		return err
	}
	if _, err := c.expect("GET", "/apiaries/apiary-missing/status", nil, 404, `"code":"not_found"`); err != nil {
		return err
	}
	if _, err := c.expect("GET", "/apiaries/apiary-north/status?asOf=not-a-time", nil, 400,
		`"code":"invalid_request"`); err != nil {
		return err
	}
	return nil
}

type amendmentView struct {
	ID        string  `json:"id"`
	Kind      string  `json:"kind"`
	Temper    *string `json:"temper"`
	MiteCount *int    `json:"miteCount"`
	Note      *string `json:"note"`
}

type conclusionView struct {
	ColonyStrength int      `json:"colonyStrength"`
	BroodPattern   string   `json:"broodPattern"`
	Temper         string   `json:"temper"`
	MiteCount      int      `json:"miteCount"`
	Note           string   `json:"note"`
	Concerns       []string `json:"concerns"`
}

type inspectionView struct {
	ID             string          `json:"id"`
	ColonyStrength int             `json:"colonyStrength"`
	BroodPattern   string          `json:"broodPattern"`
	Temper         string          `json:"temper"`
	MiteCount      int             `json:"miteCount"`
	Note           string          `json:"note"`
	Concerns       []string        `json:"concerns"`
	Effective      conclusionView  `json:"effective"`
	Amendments     []amendmentView `json:"amendments"`
}

func checkAmendInspection(c *client) error {
	if err := seedDefault(c); err != nil {
		return err
	}
	if _, err := c.expect("POST", "/hives/hive-01/inspections", map[string]any{
		"observedAt": "2026-04-10T09:00:00Z", "colonyStrength": 3, "broodPattern": "patchy",
		"temper": "aggressive", "miteCount": 12, "note": "西侧巢门观察",
	}, 201, `"inspection-000001"`, `"concerns":["mite_load","temper_risk"]`); err != nil {
		return err
	}
	// 追加之前，状态汇总按原始关注项判定。
	if _, err := c.expect("GET", "/apiaries/apiary-north/status?asOf=2026-04-12T00:00:00Z", nil, 200,
		`"concernHives":["hive-01"]`); err != nil {
		return err
	}
	// 更正一处笔误：性情实为安静。原始量测与原始关注项保留，生效结论更新。
	amendment, err := c.expect("POST", "/hives/hive-01/inspections/inspection-000001/amendments", map[string]any{
		"amendmentId": "amend-01", "kind": "correction", "temper": "calm",
		"note": "更正：性情记录笔误，实为安静",
	}, 201, `"id":"amend-01"`, `"kind":"correction"`, `"concerns":["mite_load","temper_risk"]`,
		`"concerns":["mite_load"]`)
	if err != nil {
		return err
	}
	if err := amendment.mustOrder(`"temper":"aggressive"`, `"temper":"calm"`); err != nil {
		return err
	}
	// 相同内容重放同一 amendmentId：幂等返回 200，不重复追加。
	if _, err := c.expect("POST", "/hives/hive-01/inspections/inspection-000001/amendments", map[string]any{
		"amendmentId": "amend-01", "kind": "correction", "temper": "calm",
		"note": "更正：性情记录笔误，实为安静",
	}, 200, `"id":"amend-01"`); err != nil {
		return err
	}
	// 同一 amendmentId 携带不同内容：冲突拒绝。
	if _, err := c.expect("POST", "/hives/hive-01/inspections/inspection-000001/amendments", map[string]any{
		"amendmentId": "amend-01", "kind": "correction", "temper": "nervous",
	}, 409, `"code":"conflict"`); err != nil {
		return err
	}
	// 随访：三日后复查螨计数实为 4，生效结论不再有任何关注项。
	if _, err := c.expect("POST", "/hives/hive-01/inspections/inspection-000001/amendments", map[string]any{
		"amendmentId": "amend-02", "kind": "followup", "miteCount": 4,
		"note": "三日后复查，螨计数实为 4",
	}, 201, `"id":"amend-02"`, `"kind":"followup"`, `"concerns":[]`); err != nil {
		return err
	}
	// 状态汇总按当前生效结论判定未处理关注项。
	if _, err := c.expect("GET", "/apiaries/apiary-north/status?asOf=2026-04-12T00:00:00Z", nil, 200,
		`"concernHives":[]`, `"latestConcerns":[]`); err != nil {
		return err
	}
	// 列表读回仍能还原原始记录。
	if _, err := c.expect("GET", "/hives/hive-01/inspections", nil, 200,
		`"colonyStrength":3`, `"miteCount":12`, `"concerns":["mite_load","temper_risk"]`); err != nil {
		return err
	}
	// 失败路径：未知蜂箱、未知巡检、空追加、非法类型、越界量测。
	if _, err := c.expect("POST", "/hives/hive-99/inspections/inspection-000001/amendments", map[string]any{
		"amendmentId": "amend-x1", "kind": "followup", "note": "蜂箱不存在",
	}, 404, `"code":"not_found"`); err != nil {
		return err
	}
	if _, err := c.expect("POST", "/hives/hive-01/inspections/inspection-999999/amendments", map[string]any{
		"amendmentId": "amend-x1", "kind": "followup", "note": "巡检不存在",
	}, 404, `"code":"not_found"`); err != nil {
		return err
	}
	if _, err := c.expect("GET", "/hives/hive-01/inspections/inspection-999999", nil, 404,
		`"code":"not_found"`); err != nil {
		return err
	}
	if _, err := c.expect("POST", "/hives/hive-01/inspections/inspection-000001/amendments", map[string]any{
		"amendmentId": "amend-x2", "kind": "followup",
	}, 400, `"code":"invalid_request"`); err != nil {
		return err
	}
	if _, err := c.expect("POST", "/hives/hive-01/inspections/inspection-000001/amendments", map[string]any{
		"amendmentId": "amend-x3", "kind": "review", "note": "非法类型",
	}, 400, `"code":"invalid_request"`); err != nil {
		return err
	}
	if _, err := c.expect("POST", "/hives/hive-01/inspections/inspection-000001/amendments", map[string]any{
		"amendmentId": "amend-x4", "kind": "correction", "miteCount": 99,
	}, 400, `"code":"invalid_request"`); err != nil {
		return err
	}
	// 并发重复追加同一内容：恰好一条生效，其余幂等重放。
	amendPath := "/hives/hive-01/inspections/inspection-000001/amendments"
	duplicate := map[string]any{
		"amendmentId": "amend-03", "kind": "followup", "note": "重复上报的随访",
	}
	replies, err := callConcurrently(c, 6, "POST", amendPath, func(int) any { return duplicate })
	if err != nil {
		return err
	}
	created, replayed := 0, 0
	for _, reply := range replies {
		switch reply.status {
		case 201:
			created++
		case 200:
			replayed++
		default:
			return fmt.Errorf("并发追加返回意外状态 %d：%s", reply.status, reply.text())
		}
	}
	if created != 1 || replayed != 5 {
		return fmt.Errorf("并发重复追加应恰好一条生效，实际 created=%d replayed=%d", created, replayed)
	}
	// 并发追加不同内容：全部生效且按到达顺序依次折叠。
	replies, err = callConcurrently(c, 4, "POST", amendPath, func(index int) any {
		return map[string]any{
			"amendmentId": fmt.Sprintf("amend-c%d", index+1),
			"kind":        "followup",
			"note":        fmt.Sprintf("并发随访 %d", index+1),
		}
	})
	if err != nil {
		return err
	}
	for _, reply := range replies {
		if reply.status != 201 {
			return fmt.Errorf("并发追加不同内容应全部生效，实际状态 %d：%s", reply.status, reply.text())
		}
	}
	// 最终读回：原始事实未被改写，生效结论与追加日志按顺序折叠一致。
	final, err := c.expect("GET", "/hives/hive-01/inspections/inspection-000001", nil, 200,
		`"id":"inspection-000001"`)
	if err != nil {
		return err
	}
	var view inspectionView
	if err := final.decode(&view); err != nil {
		return fmt.Errorf("解析巡检响应失败：%v", err)
	}
	if len(view.Amendments) != 7 {
		return fmt.Errorf("追加日志应包含 7 条记录，实际 %d：%s", len(view.Amendments), final.text())
	}
	if view.ColonyStrength != 3 || view.Temper != "aggressive" || view.MiteCount != 12 ||
		view.Note != "西侧巢门观察" {
		return fmt.Errorf("原始量测被追加改写：%s", final.text())
	}
	if strings.Join(view.Concerns, ",") != "mite_load,temper_risk" {
		return fmt.Errorf("原始关注项被追加改写：%v", view.Concerns)
	}
	if view.Effective.Temper != "calm" || view.Effective.MiteCount != 4 {
		return fmt.Errorf("生效结论与追加日志不一致：%+v", view.Effective)
	}
	last := view.Amendments[len(view.Amendments)-1]
	if last.Note == nil || view.Effective.Note != *last.Note {
		return fmt.Errorf("生效备注 %q 与最后一条追加 %v 不一致", view.Effective.Note, last.Note)
	}
	if len(view.Effective.Concerns) != 0 {
		return fmt.Errorf("生效结论不应再有关注项：%v", view.Effective.Concerns)
	}
	return nil
}

// callConcurrently 并发发起 count 次相同路径的请求，用于验证追加的串行化与幂等。
func callConcurrently(c *client, count int, method, path string, payload func(index int) any) ([]*response, error) {
	replies := make([]*response, count)
	errs := make([]error, count)
	var wg sync.WaitGroup
	for index := 0; index < count; index++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			reply, err := c.call(method, path, payload(i))
			if err != nil {
				errs[i] = err
				return
			}
			replies[i] = reply
		}(index)
	}
	wg.Wait()
	for _, err := range errs {
		if err != nil {
			return nil, err
		}
	}
	return replies, nil
}
