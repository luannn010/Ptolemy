package voice

import (
	"regexp"
	"strconv"
	"strings"
	"time"
)

type CommandType string

const (
	CommandSleepPC     CommandType = "sleep_pc"
	CommandSetAlarm    CommandType = "set_alarm"
	CommandSetReminder CommandType = "set_reminder"
	CommandRunShell    CommandType = "run_shell"
)

type Command struct {
	Type    CommandType
	When    time.Time
	Message string
	Shell   string
}

var (
	alarmAtRe     = regexp.MustCompile(`^set alarm at ([0-9]{1,2}):([0-9]{2})(?:\s*(am|pm))?$`)
	alarmSimpleRe = regexp.MustCompile(`^set alarm ([0-9]{1,2}):([0-9]{2})(?:\s*(am|pm))?$`)
	reminderInRe  = regexp.MustCompile(`^set reminder (.+?) in ([0-9]+) minutes?$`)
	reminderAtRe  = regexp.MustCompile(`^set reminder (.+?) at ([0-9]{1,2}):([0-9]{2})(?:\s*(am|pm))?$`)
	runShellRe    = regexp.MustCompile(`^run(?: command)? (.+)$`)
	wakePhraseRe  = regexp.MustCompile(`\bhey ptolemy\b`)
	confirmRe     = regexp.MustCompile(`^(confirm|yes do it|execute)$`)
	spaceCollapse = regexp.MustCompile(`\s+`)
)

func IsWakePhrase(text string) bool {
	return wakePhraseRe.MatchString(strings.ToLower(text))
}

// IsConfirmPhrase reports whether the spoken text is an affirmative confirmation
// used to execute a pending shell command.
func IsConfirmPhrase(text string) bool {
	return confirmRe.MatchString(normalize(text))
}

func ParseCommand(text string, now time.Time) (Command, bool) {
	text = normalize(text)
	if text == "sleep pc" || text == "turn pc to sleep" {
		return Command{Type: CommandSleepPC}, true
	}
	if m := alarmAtRe.FindStringSubmatch(text); len(m) > 0 {
		when, ok := parseClock(now, m[1], m[2], m[3])
		if !ok {
			return Command{}, false
		}
		return Command{Type: CommandSetAlarm, When: when}, true
	}
	if m := alarmSimpleRe.FindStringSubmatch(text); len(m) > 0 {
		when, ok := parseClock(now, m[1], m[2], m[3])
		if !ok {
			return Command{}, false
		}
		return Command{Type: CommandSetAlarm, When: when}, true
	}
	if m := reminderInRe.FindStringSubmatch(text); len(m) > 0 {
		minutes, err := strconv.Atoi(m[2])
		if err != nil || minutes <= 0 {
			return Command{}, false
		}
		return Command{
			Type:    CommandSetReminder,
			When:    now.Add(time.Duration(minutes) * time.Minute),
			Message: strings.TrimSpace(m[1]),
		}, true
	}
	if m := reminderAtRe.FindStringSubmatch(text); len(m) > 0 {
		when, ok := parseClock(now, m[2], m[3], m[4])
		if !ok {
			return Command{}, false
		}
		return Command{
			Type:    CommandSetReminder,
			When:    when,
			Message: strings.TrimSpace(m[1]),
		}, true
	}
	if m := runShellRe.FindStringSubmatch(text); len(m) > 0 {
		shell := strings.TrimSpace(m[1])
		if shell == "" {
			return Command{}, false
		}
		return Command{Type: CommandRunShell, Shell: shell}, true
	}
	return Command{}, false
}

func IsCommandWindowExpired(until, now time.Time) bool {
	return !until.IsZero() && (now.Equal(until) || now.After(until))
}

func normalize(input string) string {
	return strings.TrimSpace(spaceCollapse.ReplaceAllString(strings.ToLower(input), " "))
}

func parseClock(now time.Time, hhText, mmText, meridiem string) (time.Time, bool) {
	hh, err := strconv.Atoi(hhText)
	if err != nil {
		return time.Time{}, false
	}
	mm, err := strconv.Atoi(mmText)
	if err != nil || mm < 0 || mm > 59 {
		return time.Time{}, false
	}
	if meridiem != "" {
		if hh < 1 || hh > 12 {
			return time.Time{}, false
		}
		if meridiem == "pm" && hh != 12 {
			hh += 12
		}
		if meridiem == "am" && hh == 12 {
			hh = 0
		}
	} else if hh < 0 || hh > 23 {
		return time.Time{}, false
	}
	when := time.Date(now.Year(), now.Month(), now.Day(), hh, mm, 0, 0, now.Location())
	if !when.After(now) {
		when = when.Add(24 * time.Hour)
	}
	return when, true
}
