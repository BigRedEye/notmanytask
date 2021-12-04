package deadlines

import (
	"fmt"
	"math"
	"time"
)

type ScoringPolicy interface {
	Score(maxScore int, deadline time.Time, submitTime time.Time) int
}

type ScoringPolicySpec struct {
	Name   string
	Kind   string
	Policy ScoringPolicy `yaml:"-" json:"-"`
}

type ScoringGroup struct {
	Name     string
	Weight   float64
	MaxScore int `yaml:"maxScore"`
	Policy   string
}

type Scoring struct {
	Policies     []ScoringPolicySpec
	Groups       []ScoringGroup
	DefaultGroup string `yaml:"defaultGroup"`
}

////////////////////////////////////////////////////////////////////////////////

type ExponentialScore struct {
	Multiplier time.Duration
	Threshold  float64
}

func (s *ExponentialScore) Score(maxScore int, deadline time.Time, submitTime time.Time) int {
	if submitTime.Before(deadline) {
		return maxScore
	}
	deltaSeconds := submitTime.Sub(deadline).Seconds()
	exp := deltaSeconds / s.Multiplier.Seconds()
	return int(math.Max(s.Threshold, 1.0/math.Exp(exp)) * float64(maxScore))
}

////////////////////////////////////////////////////////////////////////////////

type LinearScore struct {
	After      time.Duration
	Multiplier float64
}

func (s *LinearScore) Score(maxScore int, deadline time.Time, submitTime time.Time) int {
	if submitTime.Before(deadline) {
		return maxScore
	}
	finish := deadline.Add(s.After)

	mult := s.Multiplier
	if submitTime.Before(finish) {
		delta := submitTime.Sub(deadline).Seconds() / s.After.Seconds()
		mult = (1.0 - delta) + (s.Multiplier * delta)
	}

	return int(float64(maxScore) * mult)
}

////////////////////////////////////////////////////////////////////////////////

type yamlNode struct {
	unmarshal func(interface{}) error
}

func (n *yamlNode) UnmarshalYAML(unmarshal func(interface{}) error) error {
	n.unmarshal = unmarshal
	return nil
}

func (s *ScoringPolicySpec) UnmarshalYAML(unmarshal func(interface{}) error) error {
	type S ScoringPolicySpec
	type T struct {
		S    `yaml:",inline"`
		Spec yamlNode `yaml:"spec"`
	}

	obj := &T{}
	if err := unmarshal(obj); err != nil {
		return err
	}
	*s = ScoringPolicySpec(obj.S)

	switch s.Kind {
	case "exp":
		s.Policy = new(ExponentialScore)
	case "linear":
		s.Policy = new(LinearScore)
	default:
		return fmt.Errorf("Unknown policy %s", s.Kind)
	}
	return obj.Spec.unmarshal(s.Policy)
}
