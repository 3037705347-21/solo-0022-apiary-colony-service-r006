package inspection

import (
	"time"

	"example.com/apiary-colony-service/internal/domain"
	"example.com/apiary-colony-service/internal/store"
)

// Service 接收巡检、完成范围校验并派生关注项；也处理对已有巡检的追加。
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
	concerns := DeriveConcerns(record.Effective)
	record.Concerns = concerns
	record.Effective.Concerns = append([]string{}, concerns...)
	saved, err := s.store.AddInspection(hiveID, record)
	if err != nil {
		return domain.Inspection{}, err
	}
	return saved, nil
}

// AmendInput 是一次追加的入口输入，量测字段缺省表示不更正该项。
type AmendInput struct {
	AmendmentID    string
	Kind           string
	ColonyStrength *int
	BroodPattern   *string
	Temper         *string
	MiteCount      *int
	Note           *string
}

// Amend 向已有巡检追加一条随访或更正。追加在仓储写锁内折叠进生效结论，
// 并发追加按到达顺序依次生效；同一 amendmentId 内容一致时幂等重放，
// 内容不同则返回冲突，避免重复提交产生相互矛盾的生效结论。
func (s *Service) Amend(hiveID, inspectionID string, input AmendInput) (domain.Inspection, bool, error) {
	kind, err := ParseAmendmentKind(input.Kind)
	if err != nil {
		return domain.Inspection{}, false, err
	}
	amendment, err := domain.NewAmendment(domain.AmendmentDraft{
		ID:             input.AmendmentID,
		Kind:           kind,
		ColonyStrength: input.ColonyStrength,
		BroodPattern:   optionalBroodPattern(input.BroodPattern),
		Temper:         optionalTemper(input.Temper),
		MiteCount:      input.MiteCount,
		Note:           input.Note,
		RecordedAt:     s.clock(),
	})
	if err != nil {
		return domain.Inspection{}, false, err
	}
	created := false
	updated, err := s.store.UpdateInspection(hiveID, inspectionID, func(current domain.Inspection) (domain.Inspection, error) {
		for _, existing := range current.Amendments {
			if existing.ID != amendment.ID {
				continue
			}
			if existing.SameContent(amendment) {
				return current, nil
			}
			return domain.Inspection{}, domain.Conflict(
				"巡检 %q 的追加 %q 已存在且内容不同", inspectionID, amendment.ID)
		}
		current.Amendments = append(current.Amendments, amendment)
		current.Effective = current.Effective.Apply(amendment)
		current.Effective.Concerns = DeriveConcerns(current.Effective)
		created = true
		return current, nil
	})
	if err != nil {
		return domain.Inspection{}, false, err
	}
	return updated, created, nil
}

func (s *Service) Get(hiveID, inspectionID string) (domain.Inspection, error) {
	return s.store.GetInspection(hiveID, inspectionID)
}

func (s *Service) List(hiveID string) ([]domain.Inspection, error) {
	return s.store.ListInspections(hiveID)
}

// ParseAmendmentKind 把入口层字符串收敛为受支持的追加类型。
func ParseAmendmentKind(value string) (domain.AmendmentKind, error) {
	kind := domain.AmendmentKind(domain.TrimText(value))
	if !domain.ValidAmendmentKind(kind) {
		return "", domain.Invalid(
			"kind",
			"kind 必须是 %s 或 %s",
			domain.AmendmentFollowup, domain.AmendmentCorrection,
		)
	}
	return kind, nil
}

func optionalBroodPattern(value *string) *domain.BroodPattern {
	if value == nil {
		return nil
	}
	parsed := domain.BroodPattern(domain.TrimText(*value))
	return &parsed
}

func optionalTemper(value *string) *domain.Temper {
	if value == nil {
		return nil
	}
	parsed := domain.Temper(domain.TrimText(*value))
	return &parsed
}
