package main

import (
	"context"
	"database/sql"
	"time"
)

type SeedSummary struct {
	SiteID     string       `json:"site_id"`
	AssetID    string       `json:"asset_id"`
	Procedure  string       `json:"procedure_id"`
	RunID      string       `json:"run_id"`
	ChannelIDs []string     `json:"channel_ids"`
	Ingest     IngestResult `json:"ingest"`
}

func (s *Store) SeedDemo(ctx context.Context, actor string) (SeedSummary, error) {
	started := time.Now().UTC().Add(-5 * time.Minute)
	asset := Asset{
		ID:           "asset_vibration_rig_01",
		ExternalID:   "FDE-RIG-SEA-001",
		Name:         "Seattle vibration test stand",
		Kind:         "test_stand",
		SerialNumber: "SEA-RIG-001",
		Location:     "downtown-seattle-lab",
		Tags:         []string{"hardware-test", "edge", "customer-onboarding"},
		Metadata: JSONMap{
			"network":       "airgapped-ready",
			"customer_site": "demo",
			"owner":         "field-engineering",
		},
	}
	_ = asset.Prepare()

	procedure := Procedure{
		ID:      "proc_acceptance_vibration_v1",
		Name:    "Acceptance vibration profile",
		Version: "1.0.0",
		Requirements: []string{
			"gateway publishes NDJSON samples",
			"operator validates alarm threshold behavior",
			"all event-log rows are replayable",
		},
		Steps: []ProcedureStep{
			{Name: "connect asset", Instruction: "Attach gateway to test stand and verify channel discovery."},
			{Name: "run sweep", Instruction: "Execute 30 second vibration sweep across configured band.", Expected: "Samples arrive under 250ms."},
			{Name: "verify alarm", Instruction: "Cross threshold once and confirm open alarm."},
		},
		Metadata: JSONMap{"domain": "hardware-telemetry", "handoff": "customer-onboarding"},
	}
	_ = procedure.Prepare()

	run := TestRun{
		ID:          "run_seed_acceptance_001",
		AssetID:     asset.ID,
		ProcedureID: procedure.ID,
		RunNumber:   "SEA-FDE-0001",
		Status:      "active",
		Operator:    "seed.operator",
		SiteID:      s.cfg.SiteID,
		StartedAt:   started,
		Metadata: JSONMap{
			"shift":         "acceptance",
			"deployment":    "onboarding",
			"source":        "seed",
			"telemetry_bus": "gateway-ndjson",
		},
	}
	_ = run.Prepare(s.cfg.SiteID)

	channels := []Channel{
		{
			ID:           "chan_accel_x",
			AssetID:      asset.ID,
			Name:         "acceleration_x",
			Unit:         "g",
			SampleRateHz: 1000,
			DataType:     "float64",
			AlarmMax:     ptrFloat(7.5),
			Metadata:     JSONMap{"axis": "x", "sensor": "imu-01"},
		},
		{
			ID:           "chan_case_temp",
			AssetID:      asset.ID,
			Name:         "case_temperature",
			Unit:         "celsius",
			SampleRateHz: 1,
			DataType:     "float64",
			AlarmMax:     ptrFloat(85),
			Metadata:     JSONMap{"sensor": "thermal-01"},
		},
		{
			ID:           "chan_bus_voltage",
			AssetID:      asset.ID,
			Name:         "bus_voltage",
			Unit:         "volts",
			SampleRateHz: 100,
			DataType:     "float64",
			AlarmMin:     ptrFloat(23.5),
			AlarmMax:     ptrFloat(28.5),
			Metadata:     JSONMap{"sensor": "power-rail-a"},
		},
	}
	for i := range channels {
		_ = channels[i].Prepare()
	}

	err := s.withTx(ctx, func(tx *sql.Tx) error {
		if err := upsertAsset(ctx, tx, asset); err != nil {
			return err
		}
		if err := upsertProcedure(ctx, tx, procedure); err != nil {
			return err
		}
		if err := upsertTestRun(ctx, tx, run); err != nil {
			return err
		}
		for _, channel := range channels {
			if err := upsertChannel(ctx, tx, channel); err != nil {
				return err
			}
		}
		_, err := s.appendAuditEventTx(ctx, tx, "onboarding", s.cfg.SiteID, "seeded", actor, map[string]any{
			"asset_id":    asset.ID,
			"procedure":   procedure.ID,
			"run_id":      run.ID,
			"channel_ids": []string{channels[0].ID, channels[1].ID, channels[2].ID},
		}, "")
		return err
	})
	if err != nil {
		return SeedSummary{}, err
	}

	samples := []TelemetrySample{
		{
			RunID:        run.ID,
			ChannelID:    "chan_accel_x",
			GatewayID:    "gateway-sea-01",
			Timestamp:    started.Add(1 * time.Second),
			NumericValue: ptrFloat(2.4),
			Quality:      "good",
			ReplayKey:    "seed-accel-x-001",
		},
		{
			RunID:        run.ID,
			ChannelID:    "chan_accel_x",
			GatewayID:    "gateway-sea-01",
			Timestamp:    started.Add(2 * time.Second),
			NumericValue: ptrFloat(8.1),
			Quality:      "good",
			ReplayKey:    "seed-accel-x-002",
			Metadata:     JSONMap{"phase": "threshold-check"},
		},
		{
			RunID:        run.ID,
			ChannelID:    "chan_case_temp",
			GatewayID:    "gateway-sea-01",
			Timestamp:    started.Add(3 * time.Second),
			NumericValue: ptrFloat(42.7),
			Quality:      "good",
			ReplayKey:    "seed-temp-001",
		},
		{
			RunID:        run.ID,
			ChannelID:    "chan_bus_voltage",
			GatewayID:    "gateway-sea-01",
			Timestamp:    started.Add(4 * time.Second),
			NumericValue: ptrFloat(27.2),
			Quality:      "good",
			ReplayKey:    "seed-voltage-001",
		},
	}
	ingest, err := s.IngestTelemetryBatch(ctx, samples, actor)
	if err != nil {
		return SeedSummary{}, err
	}

	return SeedSummary{
		SiteID:     s.cfg.SiteID,
		AssetID:    asset.ID,
		Procedure:  procedure.ID,
		RunID:      run.ID,
		ChannelIDs: []string{channels[0].ID, channels[1].ID, channels[2].ID},
		Ingest:     ingest,
	}, nil
}

func upsertAsset(ctx context.Context, tx *sql.Tx, asset Asset) error {
	_, err := tx.ExecContext(ctx, `INSERT INTO assets
		(id, external_id, name, kind, serial_number, location, tags, metadata, created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
		ON CONFLICT (id) DO UPDATE SET
			external_id=EXCLUDED.external_id,
			name=EXCLUDED.name,
			kind=EXCLUDED.kind,
			serial_number=EXCLUDED.serial_number,
			location=EXCLUDED.location,
			tags=EXCLUDED.tags,
			metadata=EXCLUDED.metadata,
			updated_at=EXCLUDED.updated_at`,
		asset.ID, asset.ExternalID, asset.Name, asset.Kind, asset.SerialNumber, asset.Location,
		mustJSON(asset.Tags), mustJSON(asset.Metadata), asset.CreatedAt, asset.UpdatedAt)
	return err
}

func upsertProcedure(ctx context.Context, tx *sql.Tx, procedure Procedure) error {
	_, err := tx.ExecContext(ctx, `INSERT INTO procedures
		(id, name, version, steps, requirements, metadata, created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
		ON CONFLICT (id) DO UPDATE SET
			name=EXCLUDED.name,
			version=EXCLUDED.version,
			steps=EXCLUDED.steps,
			requirements=EXCLUDED.requirements,
			metadata=EXCLUDED.metadata,
			updated_at=EXCLUDED.updated_at`,
		procedure.ID, procedure.Name, procedure.Version, mustJSON(procedure.Steps), mustJSON(procedure.Requirements),
		mustJSON(procedure.Metadata), procedure.CreatedAt, procedure.UpdatedAt)
	return err
}

func upsertTestRun(ctx context.Context, tx *sql.Tx, run TestRun) error {
	_, err := tx.ExecContext(ctx, `INSERT INTO test_runs
		(id, asset_id, procedure_id, run_number, status, operator, site_id, started_at, ended_at, metadata, created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)
		ON CONFLICT (id) DO UPDATE SET
			status=EXCLUDED.status,
			operator=EXCLUDED.operator,
			site_id=EXCLUDED.site_id,
			metadata=EXCLUDED.metadata,
			updated_at=EXCLUDED.updated_at`,
		run.ID, run.AssetID, run.ProcedureID, run.RunNumber, run.Status, run.Operator, run.SiteID,
		run.StartedAt, nullableTime(run.EndedAt), mustJSON(run.Metadata), run.CreatedAt, run.UpdatedAt)
	return err
}

func upsertChannel(ctx context.Context, tx *sql.Tx, channel Channel) error {
	_, err := tx.ExecContext(ctx, `INSERT INTO channels
		(id, asset_id, name, unit, sample_rate_hz, data_type, alarm_min, alarm_max, metadata, created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
		ON CONFLICT (id) DO UPDATE SET
			name=EXCLUDED.name,
			unit=EXCLUDED.unit,
			sample_rate_hz=EXCLUDED.sample_rate_hz,
			data_type=EXCLUDED.data_type,
			alarm_min=EXCLUDED.alarm_min,
			alarm_max=EXCLUDED.alarm_max,
			metadata=EXCLUDED.metadata,
			updated_at=EXCLUDED.updated_at`,
		channel.ID, channel.AssetID, channel.Name, channel.Unit, channel.SampleRateHz, channel.DataType,
		nullableFloat(channel.AlarmMin), nullableFloat(channel.AlarmMax), mustJSON(channel.Metadata),
		channel.CreatedAt, channel.UpdatedAt)
	return err
}

func ptrFloat(value float64) *float64 {
	return &value
}
