package expression

import (
	"fmt"
)

type int64NumericalExpression struct {
	FilterValue int64
	Op          Type
}

func (s int64NumericalExpression) Match(input interface{}) (bool, error) {
	inputValue, ok := input.(int64)
	if !ok {
		return false, fmt.Errorf("unexpected numeric search input \"%v\"", input)
	}

	switch s.Op {
	case LessThan:
		return inputValue < s.FilterValue, nil
	case LessThanEqual:
		return inputValue <= s.FilterValue, nil
	case EqualTo:
		return inputValue == s.FilterValue, nil
	case NotEqualTo:
		return inputValue != s.FilterValue, nil
	case GreaterThan:
		return inputValue > s.FilterValue, nil
	case GreaterThanEqual:
		return inputValue >= s.FilterValue, nil
	default:
		return false, fmt.Errorf("unknown op: %s", s.Op)
	}

}

type uint64NumericalExpression struct {
	FilterValue uint64
	Op          Type
}

func (u uint64NumericalExpression) Match(input interface{}) (bool, error) {
	inputValue, ok := input.(uint64)
	if !ok {
		return false, fmt.Errorf("unexpected numeric search input \"%v\"", input)
	}

	switch u.Op {
	case LessThan:
		return inputValue < u.FilterValue, nil
	case LessThanEqual:
		return inputValue <= u.FilterValue, nil
	case EqualTo:
		return inputValue == u.FilterValue, nil
	case NotEqualTo:
		return inputValue != u.FilterValue, nil
	case GreaterThan:
		return inputValue > u.FilterValue, nil
	case GreaterThanEqual:
		return inputValue >= u.FilterValue, nil

	default:
		return false, fmt.Errorf("unknown op: %s", u.Op)
	}
}
