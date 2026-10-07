package store

import (
	"fmt"
	"sync"
	"time"

	"example.com/apiary-colony-service/internal/domain"
)

// Memory 是并发安全的内存仓储。所有写入都在写锁内完成状态校验，
// 读取则通过 Snapshot 得到一致视图。
type Memory struct {
	mu sync.RWMutex

	apiaries    map[string]domain.Apiary
	hives       map[string]domain.Hive
	inspections map[string][]domain.Inspection
	amendments  map[string][]domain.InspectionAmendment
	queens      map[string][]domain.Queen

	inspectionSeq int
	amendmentSeq  int
	queenSeq      int
}

func NewMemory() *Memory {
	return &Memory{
		apiaries:    make(map[string]domain.Apiary),
		hives:       make(map[string]domain.Hive),
		inspections: make(map[string][]domain.Inspection),
		amendments:  make(map[string][]domain.InspectionAmendment),
		queens:      make(map[string][]domain.Queen),
	}
}

func (m *Memory) CreateApiary(apiary domain.Apiary) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.apiaries[apiary.ID]; ok {
		return domain.Conflict("蜂场 %q 已存在", apiary.ID)
	}
	m.apiaries[apiary.ID] = apiary
	return nil
}

func (m *Memory) GetApiary(id string) (domain.Apiary, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	apiary, ok := m.apiaries[id]
	return apiary, ok
}

func (m *Memory) CreateHive(hive domain.Hive) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.apiaries[hive.ApiaryID]; !ok {
		return domain.NotFound("蜂场", hive.ApiaryID)
	}
	if _, ok := m.hives[hive.ID]; ok {
		return domain.Conflict("蜂箱 %q 已存在", hive.ID)
	}
	m.hives[hive.ID] = hive
	return nil
}

func (m *Memory) GetHive(id string) (domain.Hive, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	hive, ok := m.hives[id]
	return hive, ok
}

// AdvanceHive 在同一把写锁内完成状态读取、迁移校验与写入，避免并发迁移产生交错结果。
func (m *Memory) AdvanceHive(hiveID string, target domain.HiveState, _ time.Time) (domain.Transition, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	hive, ok := m.hives[hiveID]
	if !ok {
		return domain.Transition{}, domain.NotFound("蜂箱", hiveID)
	}
	if hive.State == target {
		return domain.Transition{}, domain.InvalidState("蜂箱 %q 已经处于状态 %q", hiveID, target)
	}
	if !domain.CanTransitionHive(hive.State, target) {
		return domain.Transition{}, domain.InvalidState(
			"蜂箱 %q 不允许从 %q 迁移到 %q", hiveID, hive.State, target)
	}
	from := hive.State
	hive.State = target
	m.hives[hiveID] = hive
	return domain.Transition{HiveID: hiveID, From: from, To: target}, nil
}

// AddInspection 分配巡检编号、写入记录，并推进蜂箱的最近巡检时间。
// 蜂箱存在与可接受巡检的判定与写入处于同一把锁，避免退役与巡检之间的竞态。
func (m *Memory) AddInspection(hiveID string, inspection domain.Inspection) (domain.Inspection, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	hive, ok := m.hives[hiveID]
	if !ok {
		return domain.Inspection{}, domain.NotFound("蜂箱", hiveID)
	}
	if !hive.AcceptsInspection() {
		return domain.Inspection{}, domain.InvalidState("蜂箱 %q 处于 %q 状态，不接受巡检", hiveID, hive.State)
	}
	m.inspectionSeq++
	inspection.ID = fmt.Sprintf("inspection-%06d", m.inspectionSeq)
	inspection.HiveID = hiveID
	m.inspections[hiveID] = append(m.inspections[hiveID], inspection)

	observedAt := inspection.ObservedAt
	hive.LastInspectionAt = &observedAt
	m.hives[hiveID] = hive
	return inspection, nil
}

// AmendmentResult 是一次追加修订的结果：created=false 表示命中幂等键并重放既有修订。
type AmendmentResult struct {
	Amendment domain.InspectionAmendment
	View      domain.InspectionView
	Created   bool
}

// AddInspectionAmendment 在同一把写锁内完成巡检定位、修订号、幂等键、乐观版本与
// 随访语义的全部判定后追加修订。重复或并发请求因此串行化，不可能产生相互矛盾的
// 生效结论；原始巡检记录在此过程中只读，不会被改写。
//
// 追加随访或更正是对历史观察的补充而不是新观察，因此不要求蜂箱处于在役状态，
// 已退役蜂箱上的巡检仍可补充随访结论。
func (m *Memory) AddInspectionAmendment(
	hiveID, inspectionID string,
	amendment domain.InspectionAmendment,
	expectedRevision *int,
	now time.Time,
) (AmendmentResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	record, err := m.findInspectionLocked(hiveID, inspectionID)
	if err != nil {
		return AmendmentResult{}, err
	}
	existing := m.amendments[inspectionID]

	if amendment.IdempotencyKey != "" {
		if prior, ok := m.amendmentByKeyLocked(inspectionID, amendment.IdempotencyKey); ok {
			if !sameAmendmentIntent(prior, amendment) {
				return AmendmentResult{}, domain.Conflict(
					"幂等键 %q 已用于另一次追加", amendment.IdempotencyKey)
			}
			return AmendmentResult{
				Amendment: prior,
				View:      domain.ProjectInspection(record, m.amendments[inspectionID]),
				Created:   false,
			}, nil
		}
	}

	currentRevision := len(existing)
	if expectedRevision != nil && *expectedRevision != currentRevision {
		return AmendmentResult{}, domain.Conflict(
			"巡检 %q 当前修订号为 %d，与期望的 %d 不一致",
			inspectionID, currentRevision, *expectedRevision)
	}

	// 随访时间不得早于原始观察时间，避免把后补结论伪装成当场判断。
	if amendment.FollowedUpAt.Before(record.ObservedAt) {
		return AmendmentResult{}, domain.Invalid("followedUpAt",
			"随访时间不能早于巡检观察时间 %s", record.ObservedAt.Format(time.RFC3339))
	}

	if amendment.Kind == domain.AmendmentFollowUp {
		if err := validateFollowUpLocked(record, existing, amendment); err != nil {
			return AmendmentResult{}, err
		}
	}
	if amendment.Kind == domain.AmendmentCorrection {
		if effectiveNote(record, existing) == amendment.Note {
			return AmendmentResult{}, domain.Conflict("更正备注与当前生效备注相同，没有需要更正的内容")
		}
	}

	currentRevision++
	m.amendmentSeq++
	amendment.ID = fmt.Sprintf("amendment-%06d", m.amendmentSeq)
	amendment.InspectionID = inspectionID
	amendment.HiveID = hiveID
	amendment.Revision = currentRevision
	amendment.CreatedAt = now.UTC()
	amendment.ResolvedConcerns = append([]string{}, amendment.ResolvedConcerns...)
	existing = append(existing, amendment)
	m.amendments[inspectionID] = existing

	return AmendmentResult{
		Amendment: amendment,
		View:      domain.ProjectInspection(record, existing),
		Created:   true,
	}, nil
}

func (m *Memory) findInspectionLocked(hiveID, inspectionID string) (domain.Inspection, error) {
	if _, ok := m.hives[hiveID]; !ok {
		return domain.Inspection{}, domain.NotFound("蜂箱", hiveID)
	}
	items := m.inspections[hiveID]
	for _, item := range items {
		if item.ID == inspectionID {
			return item, nil
		}
	}
	return domain.Inspection{}, domain.NotFound("巡检", inspectionID)
}

func (m *Memory) amendmentByKeyLocked(inspectionID, key string) (domain.InspectionAmendment, bool) {
	for _, amendment := range m.amendments[inspectionID] {
		if amendment.IdempotencyKey == key {
			return amendment, true
		}
	}
	return domain.InspectionAmendment{}, false
}

// validateFollowUpLocked 校验随访只能关闭原始派生且当前仍生效的关注项。
func validateFollowUpLocked(
	record domain.Inspection,
	existing []domain.InspectionAmendment,
	amendment domain.InspectionAmendment,
) error {
	derived := make(map[string]bool, len(record.Concerns))
	for _, concern := range record.Concerns {
		derived[concern] = true
	}
	resolved := make(map[string]bool)
	for _, prior := range existing {
		if prior.Kind == domain.AmendmentFollowUp {
			for _, concern := range prior.ResolvedConcerns {
				resolved[concern] = true
			}
		}
	}
	for _, concern := range amendment.ResolvedConcerns {
		if !derived[concern] {
			return domain.Invalid("resolvedConcerns",
				"关注项 %q 不是巡检 %q 派生出的关注项，不能被随访关闭", concern, record.ID)
		}
		if resolved[concern] {
			return domain.Conflict("关注项 %q 已在此前的随访中关闭", concern)
		}
	}
	return nil
}

func effectiveNote(record domain.Inspection, existing []domain.InspectionAmendment) string {
	note := record.Note
	for _, amendment := range existing {
		if amendment.Kind == domain.AmendmentCorrection {
			note = amendment.Note
		}
	}
	return note
}

// sameAmendmentIntent 判断幂等重放是否针对同一次追加：类型、随访时间、
// 关闭的关注项集合与备注都必须一致。
func sameAmendmentIntent(a, b domain.InspectionAmendment) bool {
	if a.Kind != b.Kind || !a.FollowedUpAt.Equal(b.FollowedUpAt) || a.Note != b.Note {
		return false
	}
	if len(a.ResolvedConcerns) != len(b.ResolvedConcerns) {
		return false
	}
	for index := range a.ResolvedConcerns {
		if a.ResolvedConcerns[index] != b.ResolvedConcerns[index] {
			return false
		}
	}
	return true
}

// GetInspectionView 返回单条巡检的原始记录与当前生效结论。
func (m *Memory) GetInspectionView(hiveID, inspectionID string) (domain.InspectionView, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	record, err := m.findInspectionLocked(hiveID, inspectionID)
	if err != nil {
		return domain.InspectionView{}, err
	}
	return domain.ProjectInspection(record, m.amendments[inspectionID]), nil
}

// AddQueen 关闭原有产卵蜂王并写入新蜂王，两件事在同一把锁内完成，
// 保证任意时刻同一蜂箱至多只有一只 laying 蜂王。
func (m *Memory) AddQueen(
	hiveID string,
	queen domain.Queen,
	closeQueenID string,
	closeState domain.QueenState,
	now time.Time,
) (domain.Queen, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	hive, ok := m.hives[hiveID]
	if !ok {
		return domain.Queen{}, domain.NotFound("蜂箱", hiveID)
	}
	if !hive.AcceptsQueenChange() {
		return domain.Queen{}, domain.InvalidState("蜂箱 %q 处于 %q 状态，不接受换王", hiveID, hive.State)
	}
	if closeQueenID != "" {
		if err := m.closeQueenLocked(hiveID, closeQueenID, closeState, now); err != nil {
			return domain.Queen{}, err
		}
	}
	m.queenSeq++
	queen.ID = fmt.Sprintf("queen-%06d", m.queenSeq)
	queen.HiveID = hiveID
	queen.State = domain.QueenLaying
	m.queens[hiveID] = append(m.queens[hiveID], queen)

	hive.ActiveQueenID = queen.ID
	m.hives[hiveID] = hive
	return queen, nil
}

func (m *Memory) closeQueenLocked(hiveID, queenID string, state domain.QueenState, now time.Time) error {
	items := m.queens[hiveID]
	for index, item := range items {
		if item.ID != queenID {
			continue
		}
		if !item.IsLaying() {
			return domain.Conflict("蜂王 %q 已经关闭", queenID)
		}
		closedAt := now.UTC()
		item.State = state
		item.ClosedAt = &closedAt
		items[index] = item
		m.queens[hiveID] = items
		return nil
	}
	return domain.Storage("蜂箱 %q 上的蜂王 %q 记录缺失", hiveID, queenID)
}

// ListInspectionViews 返回蜂箱下全部巡检的读取视图：每条都同时携带
// 原始固化记录与当前生效结论。
func (m *Memory) ListInspectionViews(hiveID string) ([]domain.InspectionView, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if _, ok := m.hives[hiveID]; !ok {
		return nil, domain.NotFound("蜂箱", hiveID)
	}
	items := m.inspections[hiveID]
	out := make([]domain.InspectionView, 0, len(items))
	for _, item := range items {
		out = append(out, domain.ProjectInspection(item, m.amendments[item.ID]))
	}
	return out, nil
}

func (m *Memory) ListQueens(hiveID string) ([]domain.Queen, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if _, ok := m.hives[hiveID]; !ok {
		return nil, domain.NotFound("蜂箱", hiveID)
	}
	items := m.queens[hiveID]
	out := make([]domain.Queen, len(items))
	copy(out, items)
	return out, nil
}

// Snapshot 返回深拷贝的一致视图，供汇总类读取使用。
func (m *Memory) Snapshot() Snapshot {
	m.mu.RLock()
	defer m.mu.RUnlock()
	snapshot := Snapshot{
		Apiaries:               make(map[string]domain.Apiary, len(m.apiaries)),
		Hives:                  make(map[string]domain.Hive, len(m.hives)),
		InspectionsByHive:      make(map[string][]domain.Inspection, len(m.inspections)),
		AmendmentsByInspection: make(map[string][]domain.InspectionAmendment, len(m.amendments)),
		QueensByHive:           make(map[string][]domain.Queen, len(m.queens)),
	}
	for id, apiary := range m.apiaries {
		snapshot.Apiaries[id] = apiary
	}
	for id, hive := range m.hives {
		snapshot.Hives[id] = cloneHive(hive)
	}
	for id, items := range m.inspections {
		copied := make([]domain.Inspection, len(items))
		copy(copied, items)
		for index := range copied {
			copied[index].Concerns = append([]string{}, items[index].Concerns...)
		}
		snapshot.InspectionsByHive[id] = copied
	}
	for id, items := range m.amendments {
		copied := make([]domain.InspectionAmendment, len(items))
		copy(copied, items)
		for index := range copied {
			copied[index].ResolvedConcerns = append([]string{}, items[index].ResolvedConcerns...)
		}
		snapshot.AmendmentsByInspection[id] = copied
	}
	for id, items := range m.queens {
		copied := make([]domain.Queen, len(items))
		copy(copied, items)
		for index := range copied {
			if items[index].ClosedAt != nil {
				closedAt := *items[index].ClosedAt
				copied[index].ClosedAt = &closedAt
			}
		}
		snapshot.QueensByHive[id] = copied
	}
	return snapshot
}

func cloneHive(hive domain.Hive) domain.Hive {
	if hive.LastInspectionAt != nil {
		observedAt := *hive.LastInspectionAt
		hive.LastInspectionAt = &observedAt
	}
	return hive
}
