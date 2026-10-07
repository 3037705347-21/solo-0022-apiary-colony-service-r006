package httpapi

import (
	"net/http"

	"example.com/apiary-colony-service/internal/domain"
	"example.com/apiary-colony-service/internal/inspection"
)

type recordInspectionRequest struct {
	ObservedAt     string `json:"observedAt"`
	ColonyStrength int    `json:"colonyStrength"`
	BroodPattern   string `json:"broodPattern"`
	Temper         string `json:"temper"`
	MiteCount      int    `json:"miteCount"`
	Note           string `json:"note"`
}

func (s *Server) handleRecordInspection(w http.ResponseWriter, r *http.Request) {
	var request recordInspectionRequest
	if err := decodeJSON(w, r, &request); err != nil {
		writeError(w, err)
		return
	}
	observedAt, err := parseOptionalTime("observedAt", request.ObservedAt)
	if err != nil {
		writeError(w, err)
		return
	}
	input := inspection.RecordInput{
		ColonyStrength: request.ColonyStrength,
		BroodPattern:   request.BroodPattern,
		Temper:         request.Temper,
		MiteCount:      request.MiteCount,
		Note:           request.Note,
	}
	if observedAt != nil {
		input.ObservedAt = *observedAt
	}
	record, err := s.deps.Inspection.Record(r.PathValue("hiveId"), input)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, record)
}

// amendInspectionRequest 对已有巡检追加随访（follow_up）或更正（correction）。
// expectedRevision 用于乐观并发控制，idempotencyKey 用于重复提交的幂等重放。
type amendInspectionRequest struct {
	Kind             string   `json:"kind"`
	FollowedUpAt     string   `json:"followedUpAt"`
	ResolvedConcerns []string `json:"resolvedConcerns"`
	Note             string   `json:"note"`
	ExpectedRevision *int     `json:"expectedRevision"`
	IdempotencyKey   string   `json:"idempotencyKey"`
}

// amendInspectionResponse 返回追加的修订、叠加后的当前生效视图以及是否新建；
// created=false 表示命中幂等键后重放了既有修订。
type amendInspectionResponse struct {
	Amendment domain.InspectionAmendment `json:"amendment"`
	View      domain.InspectionView      `json:"view"`
	Created   bool                       `json:"created"`
}

func (s *Server) handleAmendInspection(w http.ResponseWriter, r *http.Request) {
	var request amendInspectionRequest
	if err := decodeJSON(w, r, &request); err != nil {
		writeError(w, err)
		return
	}
	followedUpAt, err := parseOptionalTime("followedUpAt", request.FollowedUpAt)
	if err != nil {
		writeError(w, err)
		return
	}
	input := inspection.AmendInput{
		Kind:             request.Kind,
		ResolvedConcerns: request.ResolvedConcerns,
		Note:             request.Note,
		ExpectedRevision: request.ExpectedRevision,
		IdempotencyKey:   request.IdempotencyKey,
	}
	if followedUpAt != nil {
		input.FollowedUpAt = *followedUpAt
	}
	result, err := s.deps.Inspection.Amend(
		r.PathValue("hiveId"), r.PathValue("inspectionId"), input)
	if err != nil {
		writeError(w, err)
		return
	}
	status := http.StatusOK
	if result.Created {
		status = http.StatusCreated
	}
	writeJSON(w, status, amendInspectionResponse{
		Amendment: result.Amendment,
		View:      result.View,
		Created:   result.Created,
	})
}

func (s *Server) handleGetInspection(w http.ResponseWriter, r *http.Request) {
	view, err := s.deps.Inspection.Get(
		r.PathValue("hiveId"), r.PathValue("inspectionId"))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, view)
}

func (s *Server) handleListInspections(w http.ResponseWriter, r *http.Request) {
	views, err := s.deps.Inspection.List(r.PathValue("hiveId"))
	if err != nil {
		writeError(w, err)
		return
	}
	if views == nil {
		views = []domain.InspectionView{}
	}
	writeJSON(w, http.StatusOK, views)
}
