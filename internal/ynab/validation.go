package ynab

import (
	"fmt"
	"regexp"
	"time"
)

var isoDate = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)

func ValidateDate(name, value string) error {
	if !isoDate.MatchString(value) {
		return fmt.Errorf("%s must use YYYY-MM-DD", name)
	}
	if _, err := time.Parse("2006-01-02", value); err != nil {
		return fmt.Errorf("%s is not a valid date", name)
	}
	return nil
}

func ValidateMonth(value string) error {
	if value == "current" {
		return nil
	}
	if err := ValidateDate("month", value); err != nil {
		return err
	}
	if value[len(value)-2:] != "01" {
		return fmt.Errorf("month must be the first day of a month")
	}
	return nil
}

func ValidateKnowledge(value *int64) error {
	if value != nil && *value < 0 {
		return fmt.Errorf("last_knowledge_of_server must be nonnegative")
	}
	return nil
}
