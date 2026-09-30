package agentcontext

import (
	"webtyp.com/fmt"
	"webtyp.com/time"
)

var weekdayNames = [7]string{"Sunday", "Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday"}

// stamp is the line shown before every user turn: when the user said it, in the users' local
// time, e.g. "[2026-09-29 Tuesday 10:00]\n". It depends only on the turn, so the prefix of the
// request never changes, and the last user turn tells the model what "now" is.
//
// The UTC offset is used to compute the local time but is not shown: measured with
// Qwen3.5-0.8B (2026-09-30), a stamp ending in "UTC-03:00" was copied into answers ("de 10:00
// UTC-03:00 hasta 18:00") and halved the correct ones (5/10 against 9/10).
func stamp(createdAt int64, utcOffsetMinutes int) string {
	local := createdAt + int64(utcOffsetMinutes)*60
	iso := time.FormatISO8601(local * 1e9) // "YYYY-MM-DDTHH:MM:SSZ" of the shifted instant = local time
	return fmt.Sprintf("[%s %s %s]\n", iso[:10], weekdayNames[time.Weekday(local)], iso[11:16])
}
