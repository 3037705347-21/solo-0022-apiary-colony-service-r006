package domain

import "time"

const MaxIdempotencyKeyRunes = 64

// InspectionAmendment 是对一条已固化巡检的追加内容，只能追加、不能改写或删除。
// 原始量测与当时派生的关注项仍保留在 Inspection 上，本结构只承载随访结论或备注更正。
type InspectionAmendment struct {
	ID               string        `json:"id"`
	InspectionID     string        `json:"inspectionId"`
	HiveID           string        `json:"hiveId"`
	Revision         int           `json:"revision"`
	Kind             AmendmentKind `json:"kind"`
	FollowedUpAt     time.Time     `json:"followedUpAt"`
	ResolvedConcerns []string      `json:"resolvedConcerns"`
	Note             string        `json:"note"`
	IdempotencyKey   string        `json:"idempotencyKey,omitempty"`
	CreatedAt        time.Time     `json:"createdAt"`
}

// NewInspectionAmendment 校验追加内容的形状；关注项语义、修订号与并发判定留给仓储层。
// correction 只允许携带更正后的备注；follow_up 通过 resolvedConcerns 关闭关注项。
func NewInspectionAmendment(
	kind AmendmentKind,
	followedUpAt time.Time,
	resolvedConcerns []string,
	note string,
	idempotencyKey string,
) (InspectionAmendment, error) {
	if !ValidAmendmentKind(kind) {
		return InspectionAmendment{}, Invalid("kind", "kind 必须是 %s 或 %s", AmendmentFollowUp, AmendmentCorrection)
	}
	if followedUpAt.IsZero() {
		return InspectionAmendment{}, Invalid("followedUpAt", "followedUpAt 不能为空")
	}
	if err := ValidateText("note", note, 0, MaxNoteRunes); err != nil {
		return InspectionAmendment{}, err
	}
	resolved, err := normalizeConcerns(resolvedConcerns)
	if err != nil {
		return InspectionAmendment{}, err
	}
	if kind == AmendmentCorrection {
		if len(resolved) > 0 {
			return InspectionAmendment{}, Invalid("resolvedConcerns", "更正备注不能关闭关注项")
		}
		if TrimText(note) == "" {
			return InspectionAmendment{}, Invalid("note", "更正备注不能为空")
		}
	}
	if err := ValidateText("idempotencyKey", idempotencyKey, 0, MaxIdempotencyKeyRunes); err != nil {
		return InspectionAmendment{}, err
	}
	return InspectionAmendment{
		Kind:             kind,
		FollowedUpAt:     followedUpAt.UTC(),
		ResolvedConcerns: resolved,
		Note:             TrimText(note),
		IdempotencyKey:   TrimText(idempotencyKey),
	}, nil
}

// normalizeConcerns 去空白、去重并按关注项固定顺序返回，保证同一份关闭集合落库形态一致。
func normalizeConcerns(values []string) ([]string, error) {
	seen := make(map[string]bool, len(values))
	ordered := make([]string, 0, len(values))
	for _, value := range values {
		name := TrimText(value)
		if name == "" {
			continue
		}
		if !ValidConcern(name) {
			return nil, Invalid("resolvedConcerns", "未知关注项 %q", name)
		}
		if seen[name] {
			continue
		}
		seen[name] = true
		ordered = append(ordered, name)
	}
	return orderConcerns(ordered), nil
}

// orderConcerns 按关注项的固定派生顺序排列，便于幂等比较与稳定输出。
func orderConcerns(values []string) []string {
	known := []string{ConcernMiteLoad, ConcernBroodGap, ConcernTemperRisk, ConcernLowStrength}
	present := make(map[string]bool, len(values))
	for _, value := range values {
		present[value] = true
	}
	ordered := make([]string, 0, len(values))
	for _, name := range known {
		if present[name] {
			ordered = append(ordered, name)
		}
	}
	return ordered
}

func ValidAmendmentKind(kind AmendmentKind) bool {
	switch kind {
	case AmendmentFollowUp, AmendmentCorrection:
		return true
	default:
		return false
	}
}

func ValidConcern(name string) bool {
	switch name {
	case ConcernMiteLoad, ConcernBroodGap, ConcernTemperRisk, ConcernLowStrength:
		return true
	default:
		return false
	}
}

// InspectionView 是巡检的读取视图：既保留原始固化记录，也给出当前生效的结论。
// 当前生效结论由全部追加修订按顺序投影得到，原始量测字段永不参与改写。
type InspectionView struct {
	Inspection
	EffectiveConcerns []string              `json:"effectiveConcerns"`
	ResolvedConcerns  []string              `json:"resolvedConcerns"`
	EffectiveNote     string                `json:"effectiveNote"`
	CurrentRevision   int                   `json:"currentRevision"`
	Amended           bool                  `json:"amended"`
	Amendments        []InspectionAmendment `json:"amendments"`
}

// ProjectInspection 按追加顺序把修订投影到原始巡检上，得到当前生效结论。
// 随访关闭关注项（重复关闭已关闭项由仓储层拒绝，这里对越界关闭保持忽略以防陈旧数据），
// 更正则用最后一条更正备注覆盖生效备注；原始记录本身不被修改。
func ProjectInspection(record Inspection, amendments []InspectionAmendment) InspectionView {
	view := InspectionView{
		Inspection:        record,
		EffectiveConcerns: append([]string{}, record.Concerns...),
		ResolvedConcerns:  []string{},
		EffectiveNote:     record.Note,
		Amendments:        append([]InspectionAmendment{}, amendments...),
	}
	resolved := make(map[string]bool)
	for _, amendment := range amendments {
		view.CurrentRevision = amendment.Revision
		switch amendment.Kind {
		case AmendmentFollowUp:
			for _, concern := range amendment.ResolvedConcerns {
				if resolved[concern] {
					continue
				}
				resolved[concern] = true
			}
		case AmendmentCorrection:
			view.EffectiveNote = amendment.Note
		}
	}
	effective := make([]string, 0, len(record.Concerns))
	for _, concern := range record.Concerns {
		if resolved[concern] {
			view.ResolvedConcerns = append(view.ResolvedConcerns, concern)
			continue
		}
		effective = append(effective, concern)
	}
	view.EffectiveConcerns = effective
	view.Amended = len(amendments) > 0
	return view
}
