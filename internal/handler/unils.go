package handler

import (
	"encoding/json"
	"net/http"

	"github.com/rs/zerolog"
)

// ответ в json
func respondWithJSON(w http.ResponseWriter, log *zerolog.Logger, code int, payload interface{}) {
	w.Header().Set("Content-Type", "application/json")

	response, err := json.Marshal(payload)
	if err != nil {
		log.Error().Err(err).Msg("ошибка кодирования ответа")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.WriteHeader(code)

	if _, err := w.Write(response); err != nil {
		log.Warn().Err(err).Msg("ошибка записи ответа")
	}
}

// ответ с ошибкой
func respondWithError(w http.ResponseWriter, log *zerolog.Logger, httpStatus int, errorCode string, message string) {
	payload := map[string]map[string]string{
		"error": {
			"code":    errorCode,
			"message": message,
		},
	}
	respondWithJSON(w, log, httpStatus, payload)
}

// декодирует json
func decodeJSON(w http.ResponseWriter, r *http.Request, target interface{}) error {
	r.Body = http.MaxBytesReader(w, r.Body, 1048576)

	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(target); err != nil {
		return err
	}

	if decoder.More() {
		return http.ErrBodyReadAfterClose
	}

	return nil
}
