// SPDX-License-Identifier: MIT

package standing

import (
	"context"
	"fmt"
	"github.com/agezt/agezt/kernel/roster"
	orders "github.com/agezt/agezt/kernel/standing"
	"strings"
)

type Reader interface {
	List() []orders.Order
	Get(string) (orders.Order, bool)
}
type Writer interface {
	AddStanding(orders.Order) (orders.Order, error)
	UpdateStanding(string, func(*orders.Order)) (orders.Order, bool, error)
	SetStandingEnabled(string, bool) (orders.Order, error)
	RemoveStanding(string) (bool, error)
}
type Host struct {
	Agent              func(string) (roster.Profile, bool)
	ManagedDirectError func(roster.Profile, string) string
}
type Service struct {
	reader Reader
	writer Writer
	host   Host
}

func New(reader Reader, writer Writer, host Host) *Service {
	return &Service{reader: reader, writer: writer, host: host}
}

type Record struct {
	orders.Order
	FrequencyWarning string `json:"frequency_warning,omitempty"`
	TargetStatus     string `json:"target_status,omitempty"`
	TargetError      string `json:"target_error,omitempty"`
}

func Project(order orders.Order) Record {
	return Record{Order: order, FrequencyWarning: FrequencyWarning(order)}
}
func FrequencyWarning(o orders.Order) string {
	hasEvent := false
	for _, t := range o.Triggers {
		if t.Type == orders.TriggerEvent {
			hasEvent = true
		}
		if t.Type == orders.TriggerCron {
			first := ""
			if fields := strings.Fields(strings.TrimSpace(t.Schedule)); len(fields) > 0 {
				first = fields[0]
			}
			if first == "*" || first == "*/1" || first == "0/1" {
				return "cron trigger may wake this standing order every minute"
			}
		}
	}
	if hasEvent && o.CooldownSec > 0 && o.CooldownSec < 15*60 {
		return "event cooldown is below the default 15m guard"
	}
	return ""
}
func (s *Service) ValidateAgent(ref string) error {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return nil
	}
	p, ok := s.host.Agent(ref)
	if !ok {
		return fmt.Errorf("unknown standing agent: %s", ref)
	}
	if p.Retired {
		return fmt.Errorf("standing agent %s is retired", p.Slug)
	}
	if !p.Enabled {
		return fmt.Errorf("standing agent %s is paused", p.Slug)
	}
	if !p.AllowsDirectCall() {
		return fmt.Errorf("standing %s", s.host.ManagedDirectError(p, "called"))
	}
	return nil
}

type ListInput struct{}
type ListOutput struct {
	Orders       []Record `json:"orders"`
	Count        int      `json:"count"`
	EnabledCount int      `json:"enabled_count"`
}

func (s *Service) List(_ context.Context, _ ListInput) (ListOutput, error) {
	rows := s.reader.List()
	out := make([]Record, 0, len(rows))
	enabled := 0
	for _, order := range rows {
		row := Project(order)
		if err := s.ValidateAgent(order.Agent); err != nil {
			row.TargetStatus = "blocked"
			row.TargetError = err.Error()
		} else if strings.TrimSpace(order.Agent) != "" {
			row.TargetStatus = "ready"
		}
		out = append(out, row)
		if order.Enabled {
			enabled++
		}
	}
	return ListOutput{Orders: out, Count: len(out), EnabledCount: enabled}, nil
}

type AddInput struct{ Order orders.Order }
type OrderOutput struct {
	Order Record `json:"order"`
}

func (s *Service) Add(_ context.Context, in AddInput) (OrderOutput, error) {
	if err := s.ValidateAgent(in.Order.Agent); err != nil {
		return OrderOutput{}, err
	}
	order, err := s.writer.AddStanding(in.Order)
	if err != nil {
		return OrderOutput{}, err
	}
	return OrderOutput{Order: Project(order)}, nil
}

type TextField struct {
	Value   string
	Present bool
}
type NumberField struct {
	Value   float64
	Present bool
}
type EditInput struct {
	ID                                             string
	Name, Plan, Agent, Mode, MaxTrust, BriefingMin TextField
	Assure, Cooldown                               NumberField
}
type EditOutput struct {
	Updated bool    `json:"updated"`
	Order   *Record `json:"order,omitempty"`
}

func (s *Service) Edit(_ context.Context, in EditInput) (EditOutput, error) {
	if in.Agent.Present {
		if err := s.ValidateAgent(in.Agent.Value); err != nil {
			return EditOutput{}, err
		}
	}
	order, found, err := s.writer.UpdateStanding(in.ID, func(o *orders.Order) {
		if in.Name.Present {
			o.Name = in.Name.Value
		}
		if in.Plan.Present {
			o.Plan = in.Plan.Value
		}
		if in.Agent.Present {
			o.Agent = strings.TrimSpace(in.Agent.Value)
		}
		if in.Mode.Present {
			o.Initiative.Mode = orders.InitiativeMode(in.Mode.Value)
		}
		if in.MaxTrust.Present {
			o.Initiative.MaxTrust = in.MaxTrust.Value
		}
		if in.BriefingMin.Present {
			o.BriefingMin = in.BriefingMin.Value
		}
		if in.Assure.Present {
			o.Assure = int(in.Assure.Value)
		}
		if in.Cooldown.Present {
			o.CooldownSec = int64(in.Cooldown.Value)
		}
	})
	if err != nil {
		return EditOutput{}, err
	}
	if !found {
		return EditOutput{Updated: false}, nil
	}
	row := Project(order)
	return EditOutput{Updated: true, Order: &row}, nil
}

type EnableInput struct {
	ID      string
	Enabled bool
}

func (s *Service) SetEnabled(_ context.Context, in EnableInput) (OrderOutput, error) {
	if in.Enabled {
		order, found := s.reader.Get(in.ID)
		if !found {
			return OrderOutput{}, fmt.Errorf("unknown standing order: %s", in.ID)
		}
		if err := s.ValidateAgent(order.Agent); err != nil {
			return OrderOutput{}, err
		}
	}
	order, err := s.writer.SetStandingEnabled(in.ID, in.Enabled)
	if err != nil {
		return OrderOutput{}, err
	}
	return OrderOutput{Order: Project(order)}, nil
}

type RemoveInput struct{ ID string }
type RemoveOutput struct {
	Removed bool   `json:"removed"`
	ID      string `json:"id"`
}

func (s *Service) Remove(_ context.Context, in RemoveInput) (RemoveOutput, error) {
	removed, err := s.writer.RemoveStanding(in.ID)
	if err != nil {
		return RemoveOutput{}, err
	}
	return RemoveOutput{Removed: removed, ID: in.ID}, nil
}
