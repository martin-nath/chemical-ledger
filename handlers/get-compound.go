package handlers

import (
	"chemical-ledger-backend/db"
	"chemical-ledger-backend/utils"
	"database/sql"
	"log/slog"
	"net/http"
)

type GetCompoundReq struct {
	Type string `json:"type"`
}

func GetCompoundHandler(w http.ResponseWriter, r *http.Request) {
	reqBody := &GetCompoundReq{
		Type: utils.GetParam(r, "type"),
	}

	const (
		TYPE_ALL       = "all"
		TYPE_HAS_ENTRY = "has_entry"
	)

	var rows *sql.Rows
	var err error

	switch reqBody.Type {
	case TYPE_ALL:
		rows, err = db.Conn.Query(`
		SELECT
			id,
			name,
			scale
		FROM compound
		ORDER BY lower_case_name ASC
		`)
	case TYPE_HAS_ENTRY:
		rows, err = db.Conn.Query(`
		SELECT
			c.id,
			c.name,
			c.scale
		FROM
			compound AS c
		WHERE
			EXISTS (SELECT 1 FROM entry AS e WHERE e.compound_id = c.id)
		ORDER BY
			c.lower_case_name ASC;
		`)
	default:
		slog.Error("Invalid compound filter type: " + reqBody.Type)
		utils.RespWithError(w, http.StatusBadRequest, utils.INVALID_COMPOUND_FILTER_TYPE)
		return
	}

	if err != nil {
		slog.Error("Error retrieving compounds: " + err.Error())
		utils.RespWithError(w, http.StatusInternalServerError, utils.COMPOUND_RETRIEVAL_ERR)
		return
	}

	defer rows.Close()

	type Compound struct {
		ID    string `json:"key"`
		Name  string `json:"name"`
		Scale string `json:"scale"`
	}

	compounds := []Compound{}
	for rows.Next() {
		var compound Compound
		err := rows.Scan(&compound.ID, &compound.Name, &compound.Scale)
		if err != nil {
			slog.Error("Error scanning compound: " + err.Error())
			utils.RespWithError(w, http.StatusInternalServerError, utils.COMPOUND_RETRIEVAL_ERR)
			return
		}
		compounds = append(compounds, compound)
	}

	utils.RespWithData(w, http.StatusOK, map[string]any{
		"compounds": compounds,
	})
}
