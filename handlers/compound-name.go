package handlers

import (
	"errors"
	"net/http"

	"github.com/martin-nath/chemical-ledger/db"
	"github.com/martin-nath/chemical-ledger/utils"
	"github.com/sirupsen/logrus"
)

// CompoundName handles the retrieval of compound names, optionally filtered by usage in entries.
func CompoundName(w http.ResponseWriter, r *http.Request) {
	if err := utils.ValidateReqMethod(r.Method, http.MethodGet, w); err != nil {
		return
	}

	compoundQuery := parseCompoundQuery(r)

	if err := validateCompoundQuery(w, compoundQuery); err != nil {
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
		logrus.Errorf("Failed to count compounds: %v", err)
		utils.JsonRes(w, http.StatusInternalServerError, &utils.Resp{Error: "Failed to count compounds."})
		return
	}

	if totalRows == 0 {
		logrus.Info("No compounds found.")
		utils.JsonRes(w, http.StatusOK, &utils.Resp{Data: []utils.CompoundInfo{}})
		return
	}

	compoundsList := make([]utils.CompoundInfo, totalRows)

	var dataQuery string
	if compoundQuery.Type == "entry" {
		dataQuery = `
			SELECT DISTINCT c.name, c.scale, c.id
			FROM entry e
			JOIN compound c ON e.compound_id = c.id;
		`
	} else {
		dataQuery = `
			SELECT name, scale, id
			FROM compound;
		`
	}

	rows, err := db.Db.Query(dataQuery)
	if err != nil {
		logrus.Errorf("Failed to retrieve compound data: %v", err)
		utils.JsonRes(w, http.StatusInternalServerError, &utils.Resp{Error: "Failed to retrieve compound data."})
		return
	}
	defer rows.Close()

	i := 0
	for rows.Next() {
		var compoundInfo utils.CompoundInfo
		err := rows.Scan(&compoundInfo.Name, &compoundInfo.Scale, &compoundInfo.Key)
		if err != nil {
			logrus.Errorf("Failed to process compound data: %v", err)
			utils.JsonRes(w, http.StatusInternalServerError, &utils.Resp{Error: "Failed to process compound data."})
			rows.Close()
			return
		}
		compoundsList[i] = compoundInfo
		i++
	}

	if err := rows.Err(); err != nil {
		logrus.Errorf("Error during compound data retrieval: %v", err)
		utils.JsonRes(w, http.StatusInternalServerError, &utils.Resp{Error: "Error during compound data retrieval."})
		return
	}
	logrus.Infof("Successfully retrieved %d compounds.", i)

	utils.JsonRes(w, http.StatusOK, &utils.Resp{Data: compoundsList})
}

// parseCompoundQuery parses the compound query parameters from the HTTP request.
func parseCompoundQuery(r *http.Request) utils.Compound {
	query := r.URL.Query()
	compound := utils.Compound{}
	compound.Type = query.Get("type")
	return compound
}

// validateCompoundQuery validates the compound query parameters.
func validateCompoundQuery(w http.ResponseWriter, compound utils.Compound) error {
	if compound.Type != "" && compound.Type != "entry" {
		logrus.Warnf("Invalid compound type: %s", compound.Type)
		utils.JsonRes(w, http.StatusBadRequest, &utils.Resp{Error: "Invalid compound type. Accepted value is 'entry' or empty."})
		return errors.New("invalid compound type")
	}
	return nil
}
