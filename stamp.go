package agentcontext

import (
	"webtyp.com/fmt"
	"webtyp.com/time"
)

var weekdayNames = [7]string{"Sunday", "Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday"}

// stamp is the line shown before every user turn: when the user said it, in the users' local
// time, e.g. "[2026-09-29 Tuesday 10:00 UTC-03:00]\n". It depends only on the turn, so the
// prefix of the request never changes, and the last user turn tells the model what "now" is.
func stamp(createdAt int64, utcOffsetMinutes int) string {
	local := createdAt + int64(utcOffsetMinutes)*60
	iso := time.FormatISO8601(local * 1e9) // "YYYY-MM-DDTHH:MM:SSZ" of the shifted instant = local time
	return fmt.Sprintf("[%s %s %s %s]\n", iso[:10], weekdayNames[time.Weekday(local)], iso[11:16], utcOffset(utcOffsetMinutes))
}

// utcOffset renders -180 as "UTC-03:00" and 330 as "UTC+05:30".
func utcOffset(minutes int) string {
	sign := "+"
	if minutes < 0 {
		sign = "-"
		minutes = -minutes
	}
	return fmt.Sprintf("UTC%s%02d:%02d", sign, minutes/60, minutes%60)
}
