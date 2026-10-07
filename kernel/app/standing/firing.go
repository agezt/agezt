// SPDX-License-Identifier: MIT

package standing

import (
	"context"
	"errors"
)

type Firing struct {
	service *Service
	fire    func(string) bool
}

func NewFiring(service *Service, fire func(string) bool) *Firing {
	return &Firing{service: service, fire: fire}
}

type FireInput struct{ ID string }
type FireOutput struct {
	Fired bool   `json:"fired"`
	ID    string `json:"id"`
}

func (s *Firing) Fire(_ context.Context, in FireInput) (FireOutput, error) {
	if s.fire == nil {
		return FireOutput{}, errors.New("standing-order firing is not available on this daemon")
	}
	order, found := s.service.reader.Get(in.ID)
	if !found {
		return FireOutput{Fired: false, ID: in.ID}, nil
	}
	if err := s.service.ValidateAgent(order.Agent); err != nil {
		return FireOutput{}, err
	}
	return FireOutput{Fired: s.fire(in.ID), ID: in.ID}, nil
}
