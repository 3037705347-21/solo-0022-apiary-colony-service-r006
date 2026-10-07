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

func (s *Server) handleListInspections(w http.ResponseWriter, r *http.Request) {
	items, err := s.deps.Inspection.List(r.PathValue("hiveId"))
	if err != nil {
		writeError(w, err)
		return
	}
	if items == nil {
		items = []domain.Inspection{}
	}
	writeJSON(w, http.StatusOK, items)
}

func (s *Server) handleGetInspection(w http.ResponseWriter, r *http.Request) {
	found, err := s.deps.Inspection.Get(r.PathValue("hiveId"), r.PathValue("inspectionId"))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, found)
}

type amendInspectionRequest struct {
	AmendmentID    string  `json:"amendmentId"`
	Kind           string  `json:"kind"`
	ColonyStrength *int    `json:"colonyStrength"`
	BroodPattern   *string `json:"broodPattern"`
	Temper         *string `json:"temper"`
	MiteCount      *int    `json:"miteCount"`
	Note           *string `json:"note"`
}

func (s *Server) handleAmendInspection(w http.ResponseWriter, r *http.Request) {
	var request amendInspectionRequest
	if err := decodeJSON(w, r, &request); err != nil {
		writeError(w, err)
		return
	}
	updated, created, err := s.deps.Inspection.Amend(
		r.PathValue("hiveId"),
		r.PathValue("inspectionId"),
		inspection.AmendInput{
			AmendmentID:    request.AmendmentID,
			Kind:           request.Kind,
			ColonyStrength: request.ColonyStrength,
			BroodPattern:   request.BroodPattern,
			Temper:         request.Temper,
			MiteCount:      request.MiteCount,
			Note:           request.Note,
		},
	)
	if err != nil {
		writeError(w, err)
		return
	}
	status := http.StatusCreated
	if !created {
		status = http.StatusOK
	}
	writeJSON(w, status, updated)
}
