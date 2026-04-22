package recurrence

import (
	"slices"
	"time"

	"github.com/yazanabuashour/openplanner/internal/domain"
)

const maxOccurrences = 2048

func NormalizeRule(rule *domain.RecurrenceRule) domain.RecurrenceRule {
	if rule == nil {
		return domain.RecurrenceRule{}
	}

	normalized := *rule
	if normalized.Interval == 0 {
		normalized.Interval = 1
	}

	return normalized
}

func ExpandTimed(anchor time.Time, rule *domain.RecurrenceRule, from, to time.Time) []time.Time {
	if rule == nil {
		if occursInRange(anchor, anchor, from, to) {
			return []time.Time{anchor}
		}

		return nil
	}

	return expandTimed(anchor, NormalizeRule(rule), from, to)
}

func ExpandTimedInLocation(anchor time.Time, rule *domain.RecurrenceRule, from, to time.Time, location *time.Location) []time.Time {
	if location == nil {
		return ExpandTimed(anchor, rule, from, to)
	}

	localAnchor := anchor.In(location)
	if rule == nil {
		if occursInRange(localAnchor, localAnchor, from, to) {
			return []time.Time{localAnchor}
		}

		return nil
	}

	return expandTimedInLocation(localAnchor, NormalizeRule(rule), from, to)
}

func ExpandDate(anchor string, rule *domain.RecurrenceRule, from, to time.Time) []string {
	anchorTime, err := parseDate(anchor)
	if err != nil {
		return nil
	}

	if rule == nil {
		if occursInRange(anchorTime, anchorTime.AddDate(0, 0, 1), from, to) {
			return []string{anchor}
		}

		return nil
	}

	return expandDate(anchorTime, anchor, NormalizeRule(rule), from, to)
}

func IncludesTimed(anchor time.Time, rule *domain.RecurrenceRule, target time.Time) bool {
	for _, occurrence := range expandTimed(anchor, NormalizeRule(rule), anchor.Add(-time.Second), target.Add(time.Second)) {
		if occurrence.Equal(target) {
			return true
		}
	}

	return false
}

func IncludesTimedInLocation(anchor time.Time, rule *domain.RecurrenceRule, target time.Time, location *time.Location) bool {
	if location == nil {
		return IncludesTimed(anchor, rule, target)
	}
	for _, occurrence := range expandTimedInLocation(anchor.In(location), NormalizeRule(rule), anchor.Add(-time.Second), target.Add(time.Second)) {
		if occurrence.Equal(target) {
			return true
		}
	}

	return false
}

func IncludesDate(anchor string, rule *domain.RecurrenceRule, target string) bool {
	targetTime, err := parseDate(target)
	if err != nil {
		return false
	}

	for _, occurrence := range expandDate(mustParseDate(anchor), anchor, NormalizeRule(rule), mustParseDate(anchor).Add(-time.Second), targetTime.Add(24*time.Hour)) {
		if occurrence == target {
			return true
		}
	}

	return false
}

func expandTimed(anchor time.Time, rule domain.RecurrenceRule, from, to time.Time) []time.Time {
	return expandTimedWithOptions(anchor, rule, from, to, false)
}

func expandTimedInLocation(anchor time.Time, rule domain.RecurrenceRule, from, to time.Time) []time.Time {
	return expandTimedWithOptions(anchor, rule, from, to, true)
}

func expandTimedWithOptions(anchor time.Time, rule domain.RecurrenceRule, from, to time.Time, localUntilDate bool) []time.Time {
	switch rule.Frequency {
	case domain.RecurrenceFrequencyDaily:
		return expandDailyTimed(anchor, rule, from, to, localUntilDate)
	case domain.RecurrenceFrequencyWeekly:
		return expandWeeklyTimed(anchor, rule, from, to, localUntilDate)
	case domain.RecurrenceFrequencyMonthly:
		return expandMonthlyTimed(anchor, rule, from, to, localUntilDate)
	default:
		return nil
	}
}

func expandDate(anchorTime time.Time, anchor string, rule domain.RecurrenceRule, from, to time.Time) []string {
	switch rule.Frequency {
	case domain.RecurrenceFrequencyDaily:
		return expandDailyDate(anchorTime, rule, from, to)
	case domain.RecurrenceFrequencyWeekly:
		return expandWeeklyDate(anchorTime, rule, from, to)
	case domain.RecurrenceFrequencyMonthly:
		return expandMonthlyDate(anchorTime, rule, from, to)
	default:
		return nil
	}
}

func expandDailyTimed(anchor time.Time, rule domain.RecurrenceRule, from, to time.Time, localUntilDate bool) []time.Time {
	var results []time.Time

	for occurrence, index := anchor, int32(1); len(results) < maxOccurrences; occurrence, index = occurrence.AddDate(0, 0, int(rule.Interval)), index+1 {
		if !withinRuleTimedBounds(occurrence, rule, index, localUntilDate) {
			break
		}
		if occursInRange(occurrence, occurrence, from, to) {
			results = append(results, occurrence)
		}
	}

	return results
}

func expandDailyDate(anchor time.Time, rule domain.RecurrenceRule, from, to time.Time) []string {
	var results []string

	for occurrence, index := anchor, int32(1); len(results) < maxOccurrences; occurrence, index = occurrence.AddDate(0, 0, int(rule.Interval)), index+1 {
		if !withinRuleDateBounds(occurrence, rule, index) {
			break
		}
		if occursInRange(occurrence, occurrence.AddDate(0, 0, 1), from, to) {
			results = append(results, occurrence.Format(time.DateOnly))
		}
	}

	return results
}

func expandWeeklyTimed(anchor time.Time, rule domain.RecurrenceRule, from, to time.Time, localUntilDate bool) []time.Time {
	weekdays := normalizedWeekdays(rule.ByWeekday, anchor.Weekday())
	weekStart := startOfWeek(anchor)
	var results []time.Time
	index := int32(0)

	for week := weekStart; len(results) < maxOccurrences; week = week.AddDate(0, 0, int(rule.Interval)*7) {
		for _, weekday := range weekdays {
			occurrence := atWeekdayTime(week, weekday, anchor)
			if occurrence.Before(anchor) {
				continue
			}

			index++
			if !withinRuleTimedBounds(occurrence, rule, index, localUntilDate) {
				return results
			}
			if occursInRange(occurrence, occurrence, from, to) {
				results = append(results, occurrence)
			}
		}
	}

	return results
}

func expandWeeklyDate(anchor time.Time, rule domain.RecurrenceRule, from, to time.Time) []string {
	weekdays := normalizedWeekdays(rule.ByWeekday, anchor.Weekday())
	weekStart := startOfWeek(anchor)
	var results []string
	index := int32(0)

	for week := weekStart; len(results) < maxOccurrences; week = week.AddDate(0, 0, int(rule.Interval)*7) {
		for _, weekday := range weekdays {
			occurrence := atWeekdayDate(week, weekday)
			if occurrence.Before(anchor) {
				continue
			}

			index++
			if !withinRuleDateBounds(occurrence, rule, index) {
				return results
			}
			if occursInRange(occurrence, occurrence.AddDate(0, 0, 1), from, to) {
				results = append(results, occurrence.Format(time.DateOnly))
			}
		}
	}

	return results
}

func expandMonthlyTimed(anchor time.Time, rule domain.RecurrenceRule, from, to time.Time, localUntilDate bool) []time.Time {
	days := normalizedMonthDays(rule.ByMonthDay, anchor.Day())
	var results []time.Time
	index := int32(0)

	for month := time.Date(anchor.Year(), anchor.Month(), 1, anchor.Hour(), anchor.Minute(), anchor.Second(), anchor.Nanosecond(), anchor.Location()); len(results) < maxOccurrences; month = month.AddDate(0, int(rule.Interval), 0) {
		for _, day := range days {
			occurrence, ok := atMonthDayTime(month, day)
			if !ok || occurrence.Before(anchor) {
				continue
			}

			index++
			if !withinRuleTimedBounds(occurrence, rule, index, localUntilDate) {
				return results
			}
			if occursInRange(occurrence, occurrence, from, to) {
				results = append(results, occurrence)
			}
		}
	}

	return results
}

func expandMonthlyDate(anchor time.Time, rule domain.RecurrenceRule, from, to time.Time) []string {
	days := normalizedMonthDays(rule.ByMonthDay, anchor.Day())
	var results []string
	index := int32(0)

	for month := time.Date(anchor.Year(), anchor.Month(), 1, 0, 0, 0, 0, time.UTC); len(results) < maxOccurrences; month = month.AddDate(0, int(rule.Interval), 0) {
		for _, day := range days {
			occurrence, ok := atMonthDayDate(month, day)
			if !ok || occurrence.Before(anchor) {
				continue
			}

			index++
			if !withinRuleDateBounds(occurrence, rule, index) {
				return results
			}
			if occursInRange(occurrence, occurrence.AddDate(0, 0, 1), from, to) {
				results = append(results, occurrence.Format(time.DateOnly))
			}
		}
	}

	return results
}

func occursInRange(start, end, from, to time.Time) bool {
	return start.Before(to) && !end.Before(from)
}

func withinRuleTimedBounds(occurrence time.Time, rule domain.RecurrenceRule, index int32, localUntilDate bool) bool {
	if rule.Count != nil && index > *rule.Count {
		return false
	}
	if rule.UntilAt != nil && occurrence.After(*rule.UntilAt) {
		return false
	}
	if rule.UntilDate != nil {
		untilDate, err := parseDate(*rule.UntilDate)
		if err != nil {
			return false
		}
		untilEnd := untilDate.AddDate(0, 0, 1)
		if localUntilDate {
			untilEnd = time.Date(untilDate.Year(), untilDate.Month(), untilDate.Day()+1, 0, 0, 0, 0, occurrence.Location())
		}
		if !occurrence.Before(untilEnd) {
			return false
		}
	}

	return true
}

func withinRuleDateBounds(occurrence time.Time, rule domain.RecurrenceRule, index int32) bool {
	if rule.Count != nil && index > *rule.Count {
		return false
	}
	if rule.UntilDate != nil {
		untilDate, err := parseDate(*rule.UntilDate)
		if err != nil || occurrence.After(untilDate) {
			return false
		}
	}
	if rule.UntilAt != nil {
		untilDate := time.Date(rule.UntilAt.Year(), rule.UntilAt.Month(), rule.UntilAt.Day(), 0, 0, 0, 0, time.UTC)
		if occurrence.After(untilDate) {
			return false
		}
	}

	return true
}

func normalizedWeekdays(values []domain.Weekday, fallback time.Weekday) []domain.Weekday {
	if len(values) == 0 {
		return []domain.Weekday{fromTimeWeekday(fallback)}
	}

	weekdays := slices.Clone(values)
	slices.SortFunc(weekdays, func(left, right domain.Weekday) int {
		return weekdayOffset(left) - weekdayOffset(right)
	})

	return weekdays
}

func normalizedMonthDays(values []int32, fallback int) []int {
	if len(values) == 0 {
		return []int{fallback}
	}

	days := make([]int, 0, len(values))
	for _, day := range values {
		days = append(days, int(day))
	}
	slices.Sort(days)

	return days
}

func startOfWeek(input time.Time) time.Time {
	offset := (int(input.Weekday()) + 6) % 7
	return time.Date(input.Year(), input.Month(), input.Day()-offset, input.Hour(), input.Minute(), input.Second(), input.Nanosecond(), input.Location())
}

func atWeekdayTime(weekStart time.Time, weekday domain.Weekday, anchor time.Time) time.Time {
	return weekStart.AddDate(0, 0, weekdayOffset(weekday))
}

func atWeekdayDate(weekStart time.Time, weekday domain.Weekday) time.Time {
	return time.Date(weekStart.Year(), weekStart.Month(), weekStart.Day()+weekdayOffset(weekday), 0, 0, 0, 0, time.UTC)
}

func atMonthDayTime(month time.Time, day int) (time.Time, bool) {
	date := time.Date(month.Year(), month.Month(), day, month.Hour(), month.Minute(), month.Second(), month.Nanosecond(), month.Location())
	return date, date.Month() == month.Month()
}

func atMonthDayDate(month time.Time, day int) (time.Time, bool) {
	date := time.Date(month.Year(), month.Month(), day, 0, 0, 0, 0, time.UTC)
	return date, date.Month() == month.Month()
}

func parseDate(value string) (time.Time, error) {
	return time.Parse(time.DateOnly, value)
}

func mustParseDate(value string) time.Time {
	parsed, err := parseDate(value)
	if err != nil {
		return time.Time{}
	}

	return parsed
}

func fromTimeWeekday(value time.Weekday) domain.Weekday {
	switch value {
	case time.Monday:
		return domain.WeekdayMonday
	case time.Tuesday:
		return domain.WeekdayTuesday
	case time.Wednesday:
		return domain.WeekdayWednesday
	case time.Thursday:
		return domain.WeekdayThursday
	case time.Friday:
		return domain.WeekdayFriday
	case time.Saturday:
		return domain.WeekdaySaturday
	default:
		return domain.WeekdaySunday
	}
}

func weekdayOffset(value domain.Weekday) int {
	switch value {
	case domain.WeekdayMonday:
		return 0
	case domain.WeekdayTuesday:
		return 1
	case domain.WeekdayWednesday:
		return 2
	case domain.WeekdayThursday:
		return 3
	case domain.WeekdayFriday:
		return 4
	case domain.WeekdaySaturday:
		return 5
	default:
		return 6
	}
}
