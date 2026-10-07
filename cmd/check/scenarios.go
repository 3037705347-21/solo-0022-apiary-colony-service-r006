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

// checkAmendInspection 覆盖对已有巡检追加随访/更正：原始量测与原备注保留、
// 当前生效结论随之变化、汇总按生效结论判断，以及重复/并发追加不会产生矛盾结论。
func checkAmendInspection(c *client) error {
	if err := seedDefault(c); err != nil {
		return err
	}
	// 一次群势 2、螨计数 12 的巡检，原始派生出 mite_load 与 low_strength。
	if _, err := c.expect("POST", "/hives/hive-01/inspections", map[string]any{
		"observedAt": "2026-04-10T09:00:00Z", "colonyStrength": 2, "broodPattern": "solid",
		"temper": "calm", "miteCount": 12, "note": "西侧巢门观察",
	}, 201, `"id":"inspection-000001"`, `"concerns":["mite_load","low_strength"]`); err != nil {
		return err
	}
	amend := "/hives/hive-01/inspections/inspection-000001/amendments"

	// 随访确认螨害已处理，关闭 mite_load；原始量测与原始关注项必须原样保留，
	// 当前生效关注项只剩 low_strength，currentRevision 推进到 1。
	if _, err := c.expect("POST", amend, map[string]any{
		"kind": "follow_up", "followedUpAt": "2026-04-13T10:00:00Z",
		"resolvedConcerns": []string{"mite_load"}, "note": "治螨后复查",
		"idempotencyKey": "fu-mite-20260413",
	}, 201,
		`"revision":1`, `"kind":"follow_up"`,
		`"colonyStrength":2`, `"miteCount":12`,
		`"concerns":["mite_load","low_strength"]`,
		`"resolvedConcerns":["mite_load"]`,
		`"effectiveConcerns":["low_strength"]`,
		`"note":"西侧巢门观察"`,
		`"currentRevision":1`, `"amended":true`,
	); err != nil {
		return err
	}

	// 幂等重放：同一幂等键、同一份内容返回 200 且不产生新修订。
	if reply, err := c.expect("POST", amend, map[string]any{
		"kind": "follow_up", "followedUpAt": "2026-04-13T10:00:00Z",
		"resolvedConcerns": []string{"mite_load"}, "note": "治螨后复查",
		"idempotencyKey": "fu-mite-20260413",
	}, 200, `"created":false`, `"revision":1`); err != nil {
		return err
	} else if !strings.Contains(reply.text(), `"id":"amendment-000001"`) {
		return fmt.Errorf("幂等重放应返回既有修订 amendment-000001：%s", reply.text())
	}

	// 幂等键复用但内容不同：冲突，不能把同一个键挂到相互矛盾的结论上。
	if _, err := c.expect("POST", amend, map[string]any{
		"kind": "follow_up", "followedUpAt": "2026-04-14T10:00:00Z",
		"resolvedConcerns": []string{"low_strength"}, "note": "另一次随访",
		"idempotencyKey": "fu-mite-20260413",
	}, 409, `"code":"conflict"`); err != nil {
		return err
	}

	// 重复关闭同一关注项：冲突。
	if _, err := c.expect("POST", amend, map[string]any{
		"kind": "follow_up", "resolvedConcerns": []string{"mite_load"},
	}, 409, `"code":"conflict"`); err != nil {
		return err
	}
	// 关闭原始记录里不存在的关注项：校验错误。
	if _, err := c.expect("POST", amend, map[string]any{
		"kind": "follow_up", "resolvedConcerns": []string{"brood_gap"},
	}, 400, `"code":"invalid_request"`); err != nil {
		return err
	}
	// 更正备注不能携带关注项，且更正备注不能为空。
	if _, err := c.expect("POST", amend, map[string]any{
		"kind": "correction", "resolvedConcerns": []string{"low_strength"},
		"note": "误带关注项",
	}, 400, `"code":"invalid_request"`); err != nil {
		return err
	}
	if _, err := c.expect("POST", amend, map[string]any{
		"kind": "correction", "note": "",
	}, 400, `"code":"invalid_request"`); err != nil {
		return err
	}

	// 更正一处笔误：原始 note 保留，effectiveNote 变为更正文本。
	if _, err := c.expect("POST", amend, map[string]any{
		"kind": "correction", "note": "西坡巢门观察",
	}, 201,
		`"revision":2`, `"kind":"correction"`,
		`"note":"西侧巢门观察"`,
		`"effectiveNote":"西坡巢门观察"`,
		`"effectiveConcerns":["low_strength"]`,
	); err != nil {
		return err
	}

	// 乐观并发：基于过期修订号（1，实际已为 2）追加必须失败。
	if _, err := c.expect("POST", amend, map[string]any{
		"kind": "follow_up", "resolvedConcerns": []string{"low_strength"},
		"expectedRevision": 1,
	}, 409, `"code":"conflict"`); err != nil {
		return err
	}

	// 单条读取既能还原原始记录，也能看到当前生效结论。
	if _, err := c.expect("GET",
		"/hives/hive-01/inspections/inspection-000001", nil, 200,
		`"note":"西侧巢门观察"`, `"effectiveNote":"西坡巢门观察"`,
		`"concerns":["mite_load","low_strength"]`,
		`"resolvedConcerns":["mite_load"]`,
		`"effectiveConcerns":["low_strength"]`,
		`"currentRevision":2`,
	); err != nil {
		return err
	}
	// 列表同样给出原始与生效两套结论。
	if _, err := c.expect("GET", "/hives/hive-01/inspections", nil, 200,
		`"note":"西侧巢门观察"`, `"effectiveNote":"西坡巢门观察"`,
		`"effectiveConcerns":["low_strength"]`); err != nil {
		return err
	}

	// 随访关闭最后一个未处理关注项后，状态汇总不应再把该蜂箱算作关注蜂箱。
	if _, err := c.expect("POST", amend, map[string]any{
		"kind": "follow_up", "resolvedConcerns": []string{"low_strength"},
		"expectedRevision": 2,
	}, 201, `"revision":3`, `"effectiveConcerns":[]`); err != nil {
		return err
	}
	report, err := c.expect("GET", "/apiaries/apiary-north/status?asOf=2026-04-14T00:00:00Z", nil, 200,
		`"concernHives":[]`, `"latestConcerns":[]`)
	if err != nil {
		return err
	}
	if strings.Contains(report.text(), `"hive-01"`) &&
		strings.Contains(report.text(), `"concernHives":["hive-01"]`) {
		return fmt.Errorf("随访已关闭全部关注项，蜂箱不应再出现在 concernHives：%s", report.text())
	}

	// 已退役蜂箱不能再提交新巡检，但对历史巡检补充随访仍应成功。
	if _, err := c.expect("POST", "/hives/hive-01/state", map[string]any{"target": "retired"}, 200,
		`"to":"retired"`); err != nil {
		return err
	}
	if _, err := c.expect("POST", amend, map[string]any{
		"kind": "follow_up", "note": "退役前的补记",
	}, 201, `"revision":4`); err != nil {
		return err
	}

	// 未知对象与非法随访时间。
	if _, err := c.expect("POST",
		"/hives/hive-missing/inspections/inspection-000001/amendments",
		map[string]any{"kind": "correction", "note": "x"},
		404, `"code":"not_found"`); err != nil {
		return err
	}
	if _, err := c.expect("POST", amend, map[string]any{
		"kind": "follow_up", "followedUpAt": "2026-04-01T00:00:00Z",
	}, 400, `"code":"invalid_request"`); err != nil {
		return err
	}
	if _, err := c.expect("POST", amend, map[string]any{
		"kind": "typo",
	}, 400, `"code":"invalid_request"`); err != nil {
		return err
	}

	// 并发追加：同一起点（修订 4）的两个更正，恰好一个成功、一个版本冲突，
	// 最终只有一条成为生效结论，不会出现互相覆盖。
	results := make(chan int, 2)
	var wg sync.WaitGroup
	fire := func(note string, rev int) {
		defer wg.Done()
		reply, err := c.call("POST", amend, map[string]any{
			"kind": "correction", "note": note, "expectedRevision": rev,
		})
		if err != nil {
			results <- 0
			return
		}
		results <- reply.status
	}
	wg.Add(2)
	go fire("并发更正-A", 4)
	go fire("并发更正-B", 4)
	wg.Wait()
	close(results)
	created, conflicts := 0, 0
	for status := range results {
		switch status {
		case 201:
			created++
		case 409:
			conflicts++
		default:
			return fmt.Errorf("并发追加返回了未预期的状态码 %d", status)
		}
	}
	if created != 1 || conflicts != 1 {
		return fmt.Errorf("并发追加期望 1 次成功 1 次冲突，实际成功 %d 次冲突 %d 次", created, conflicts)
	}
	final, err := c.expect("GET",
		"/hives/hive-01/inspections/inspection-000001", nil, 200, `"currentRevision":5`)
	if err != nil {
		return err
	}
	winsA := strings.Contains(final.text(), `"effectiveNote":"并发更正-A"`)
	winsB := strings.Contains(final.text(), `"effectiveNote":"并发更正-B"`)
	if winsA == winsB {
		return fmt.Errorf("并发追加后必须恰好一个更正生效：%s", final.text())
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
