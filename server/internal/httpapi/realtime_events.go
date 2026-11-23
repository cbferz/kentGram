package httpapi

import (
	"net/http"
	"strconv"

	"github.com/cbferz/kentgram/server/internal/store"
)

func (a *API) realtimeEvents(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	since := int64(0)
	if raw := r.URL.Query().Get("since"); raw != "" {
		value, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || value < 0 {
			writeError(w, http.StatusBadRequest, "invalid_realtime_cursor")
			return
		}
		since = value
	}
	events, cursor, highWater, err := a.db.RealtimeBatch(r.Context(), userID(r), since, 100)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "realtime_sync_failed")
		return
	}
	if events == nil {
		events = make([]store.RealtimeEvent, 0)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"events": events,
		"cursor": cursor,
		"high_water": highWater,
	})
}
