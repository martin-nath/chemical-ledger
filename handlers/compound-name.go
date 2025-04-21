package handlers

import (
	"database/sql"
	"errors"
	"net/http"

	"github.com/martin-nath/chemical-ledger/db"
	"github.com/martin-nath/chemical-ledger/utils"
)

func SetDatabase(database *sql.DB) {
	db.Db = database
}

type Compound struct {
	Type string `json:"type"`
}

type CompoundInfo struct {
	Name string `json:"name"`
	Key  string `json:"key"`
}

func parseCompoundQuery(r *http.Request) Compound {
	query := r.URL.Query()
	compound := Compound{}
	compound.Type = query.Get("type")
	return compound
}

func validateCompoundQuery(w http.ResponseWriter, compound Compound) error {
	if compound.Type != "" && compound.Type != "entry" {
		utils.JsonRes(w, http.StatusBadRequest, &utils.Resp{Error: "Invalid compound type. Accepted value is 'entry' or empty."})
		return errors.New("invalid compound type")
	}
	return nil
}

func CompoundName(w http.ResponseWriter, r *http.Request) {
	if err := utils.ValidateReqMethod(r.Method, http.MethodGet, w); err != nil {
		return
	}

	compoundQuery := parseCompoundQuery(r)

	if err := validateCompoundQuery(w, compoundQuery); err != nil {
		return
	}

	if db.Db == nil {
		utils.JsonRes(w, http.StatusInternalServerError, &utils.Resp{Error: "Database connection not initialized."})
		return
	}

	var countQuery string
	if compoundQuery.Type == "entry" {
		countQuery = `
			SELECT COUNT(DISTINCT c.id)
			FROM entry e
			JOIN compound c ON e.compound_id = c.id;
		`
	} else {
		countQuery = `
			SELECT COUNT(*)
			FROM compound;
		`
	}

	var totalRows int
	err := db.Db.QueryRow(countQuery).Scan(&totalRows)
	if err != nil {
		utils.JsonRes(w, http.StatusInternalServerError, &utils.Resp{Error: "Failed to count compounds."})
		return
	}

	if totalRows == 0 {
		utils.JsonRes(w, http.StatusOK, &utils.Resp{Data: []CompoundInfo{}})
		return
	}

	compoundsList := make([]CompoundInfo, totalRows)

	var dataQuery string
	if compoundQuery.Type == "entry" {
		dataQuery = `
			SELECT DISTINCT c.name, c.id
			FROM entry e
			JOIN compound c ON e.compound_id = c.id;
		`
	} else {
		dataQuery = `
			SELECT name, id
			FROM compound;
		`
	}

	rows, err := db.Db.Query(dataQuery)
	if err != nil {
		utils.JsonRes(w, http.StatusInternalServerError, &utils.Resp{Error: "Failed to retrieve compound data."})
		return
	}
	defer rows.Close()

	i := 0
	for rows.Next() {
		if i >= totalRows {
			break
		}
		var compoundInfo CompoundInfo
		err := rows.Scan(&compoundInfo.Name, &compoundInfo.Key)
		if err != nil {
			utils.JsonRes(w, http.StatusInternalServerError, &utils.Resp{Error: "Failed to process compound data."})
			rows.Close()
			return
		}
		compoundsList[i] = compoundInfo
		i++
	}

	if err := rows.Err(); err != nil {
		utils.JsonRes(w, http.StatusInternalServerError, &utils.Resp{Error: "Error during compound data retrieval."})
		return
	}

	utils.JsonRes(w, http.StatusOK, &utils.Resp{Data: compoundsList})
}
