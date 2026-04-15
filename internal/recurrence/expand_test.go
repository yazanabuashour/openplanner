package recurrence

import (
	"testing"
	"time"

	"github.com/yazanabuashour/openplanner/internal/domain"
)

func TestExpandTimedDailyInterval(t *testing.T) {
	t.Parallel()

	anchor := time.Date(2026, 4, 15, 9, 0, 0, 0, time.UTC)
	count := int32(3)
	occurrences := ExpandTimed(anchor, &domain.RecurrenceRule{
		Frequency: domain.RecurrenceFrequencyDaily,
		Interval:  2,
		Count:     &count,
	}, time.Date(2026, 4, 15, 0, 0, 0, 0, time.UTC), time.Date(2026, 4, 21, 0, 0, 0, 0, time.UTC))

	if len(occurrences) != 3 {
		t.Fatalf("ExpandTimed() returned %d occurrences, want 3", len(occurrences))
	}
	if occurrences[1].Format(time.RFC3339) != "2026-04-17T09:00:00Z" {
		t.Fatalf("second occurrence = %s", occurrences[1].Format(time.RFC3339))
	}
}

func TestExpandDateWeeklyByWeekday(t *testing.T) {
	t.Parallel()

	occurrences := ExpandDate("2026-04-13", &domain.RecurrenceRule{
		Frequency: domain.RecurrenceFrequencyWeekly,
		ByWeekday: []domain.Weekday{domain.WeekdayMonday, domain.WeekdayWednesday},
		Count:     int32Ptr(4),
	}, time.Date(2026, 4, 13, 0, 0, 0, 0, time.UTC), time.Date(2026, 4, 24, 0, 0, 0, 0, time.UTC))

	want := []string{"2026-04-13", "2026-04-15", "2026-04-20", "2026-04-22"}
	if len(occurrences) != len(want) {
		t.Fatalf("ExpandDate() returned %d occurrences, want %d", len(occurrences), len(want))
	}
	for index, occurrence := range occurrences {
		if occurrence != want[index] {
			t.Fatalf("occurrence[%d] = %s, want %s", index, occurrence, want[index])
		}
	}
}

func TestExpandTimedUntilDateExcludesNextMidnight(t *testing.T) {
	t.Parallel()

	untilDate := "2026-04-15"
	occurrences := ExpandTimed(time.Date(2026, 4, 15, 0, 0, 0, 0, time.UTC), &domain.RecurrenceRule{
		Frequency: domain.RecurrenceFrequencyDaily,
		UntilDate: &untilDate,
	}, time.Date(2026, 4, 15, 0, 0, 0, 0, time.UTC), time.Date(2026, 4, 17, 0, 0, 0, 0, time.UTC))

	if len(occurrences) != 1 {
		t.Fatalf("ExpandTimed() returned %d occurrences, want 1", len(occurrences))
	}
	if occurrences[0].Format(time.RFC3339) != "2026-04-15T00:00:00Z" {
		t.Fatalf("first occurrence = %s", occurrences[0].Format(time.RFC3339))
	}
}

func TestExpandDateMonthlySkipsMissingMonthDays(t *testing.T) {
	t.Parallel()

	occurrences := ExpandDate("2026-01-31", &domain.RecurrenceRule{
		Frequency:  domain.RecurrenceFrequencyMonthly,
		ByMonthDay: []int32{31},
		Count:      int32Ptr(3),
	}, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC))

	want := []string{"2026-01-31", "2026-03-31"}
	if len(occurrences) != len(want) {
		t.Fatalf("ExpandDate() returned %d occurrences, want %d", len(occurrences), len(want))
	}
	for index, occurrence := range occurrences {
		if occurrence != want[index] {
			t.Fatalf("occurrence[%d] = %s, want %s", index, occurrence, want[index])
		}
	}
}

func int32Ptr(value int32) *int32 {
	return &value
}
