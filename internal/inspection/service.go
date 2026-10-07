package inspection

import (
	"time"

	"example.com/apiary-colony-service/internal/domain"
	"example.com/apiary-colony-service/internal/store"
)

// Service 接收巡检、完成范围校验并派生关注项，并允许对已有巡检追加随访或更正。
type Service struct {
	store *store.Memory
	clock func() time.Time
}

func New(memory *store.Memory, clock func() time.Time) *Service {
	if clock == nil {
		clock = time.Now
	}
	return &Service{store: memory, clock: clock}
}

type RecordInput struct {
	ObservedAt     time.Time
	ColonyStrength int
	BroodPattern   string
	Temper         string
	MiteCount      int
	Note           string
}

func (s *Service) Record(hiveID string, input RecordInput) (domain.Inspection, error) {
	observedAt := input.ObservedAt
	if observedAt.IsZero() {
		observedAt = s.clock()
	}
	record, err := domain.NewInspection(
		hiveID,
		observedAt,
		input.ColonyStrength,
		domain.BroodPattern(domain.TrimText(input.BroodPattern)),
		domain.Temper(domain.TrimText(input.Temper)),
		input.MiteCount,
		input.Note,
	)
	if err != nil {
		return domain.Inspection{}, err
	}
	record.Concerns = DeriveConcerns(record)
	saved, err := s.store.AddInspection(hiveID, record)
	if err != nil {
		return domain.Inspection{}, err
	}
	return saved, nil
}

// AmendInput 是对已有巡检的一次追加。ExpectedRevision 非空时启用乐观并发控制，
// IdempotencyKey 非空时对重复提交做幂等重放。
type AmendInput struct {
	Kind             string
	FollowedUpAt     time.Time
	ResolvedConcerns []string
	Note             string
	ExpectedRevision *int
	IdempotencyKey   string
}

// AmendResult 返回追加的修订、巡检当前生效视图以及是否新建（false 表示幂等重放）。
type AmendResult struct {
	Amendment domain.InspectionAmendment
	View      domain.InspectionView
	Created   bool
}

func (s *Service) Amend(hiveID, inspectionID string, input AmendInput) (AmendResult, error) {
	kind, err := ParseAmendmentKind(input.Kind)
	if err != nil {
		return AmendResult{}, err
	}
	followedUpAt := input.FollowedUpAt
	if followedUpAt.IsZero() {
		followedUpAt = s.clock()
	}
	amendment, err := domain.NewInspectionAmendment(
		kind,
		followedUpAt,
		input.ResolvedConcerns,
		input.Note,
		input.IdempotencyKey,
	)
	if err != nil {
		return AmendResult{}, err
	}
	result, err := s.store.AddInspectionAmendment(
		hiveID, inspectionID, amendment, input.ExpectedRevision, s.clock())
	if err != nil {
		return AmendResult{}, err
	}
	return AmendResult{Amendment: result.Amendment, View: result.View, Created: result.Created}, nil
}

func (s *Service) Get(hiveID, inspectionID string) (domain.InspectionView, error) {
	return s.store.GetInspectionView(hiveID, inspectionID)
}

// List 返回蜂箱下全部巡检的读取视图：原始记录与当前生效结论一并给出。
func (s *Service) List(hiveID string) ([]domain.InspectionView, error) {
	return s.store.ListInspectionViews(hiveID)
}

func ParseAmendmentKind(value string) (domain.AmendmentKind, error) {
	kind := domain.AmendmentKind(domain.TrimText(value))
	if !domain.ValidAmendmentKind(kind) {
		return "", domain.Invalid("kind", "kind 必须是 %s 或 %s",
			domain.AmendmentFollowUp, domain.AmendmentCorrection)
	}
	return kind, nil
}
