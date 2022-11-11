// Copyright © 2022 Ory Corp
// SPDX-License-Identifier: Apache-2.0

package cli

import (
	"github.com/ory/hydra/driver"
	"github.com/ory/x/configx"
	"github.com/ory/x/servicelocatorx"
)

type Handler struct {
	Migration *MigrateHandler
	Janitor   *JanitorHandler
}

func NewHandler(slOpts []servicelocatorx.Option, dOpts []driver.OptionsModifier, cOpts []configx.OptionModifier) *Handler {
	return &Handler{
		Migration: newMigrateHandler(),
		Janitor:   NewJanitorHandler(slOpts, dOpts, cOpts),
	}
}
