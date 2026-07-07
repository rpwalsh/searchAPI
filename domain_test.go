package main

import "testing"

func TestAssetPrepareRequiresNameAndKind(t *testing.T) {
	var asset Asset
	if err := asset.Prepare(); err == nil {
		t.Fatal("expected missing name error")
	}

	asset.Name = "Thermal Chamber"
	if err := asset.Prepare(); err == nil {
		t.Fatal("expected missing kind error")
	}

	asset.Kind = "environmental_chamber"
	if err := asset.Prepare(); err != nil {
		t.Fatalf("expected valid asset, got %v", err)
	}
	if asset.ID == "" {
		t.Fatal("expected generated asset id")
	}
}

func TestTelemetrySamplePrepareRequiresValue(t *testing.T) {
	sample := TelemetrySample{
		RunID:     "run_1",
		ChannelID: "chan_1",
	}
	if err := sample.Prepare(); err == nil {
		t.Fatal("expected missing value error")
	}

	value := 42.7
	sample.NumericValue = &value
	if err := sample.Prepare(); err != nil {
		t.Fatalf("expected valid sample, got %v", err)
	}
	if sample.Quality != "good" {
		t.Fatalf("expected default quality good, got %q", sample.Quality)
	}
	if sample.ID == "" {
		t.Fatal("expected generated sample id")
	}
}

func TestChannelPrepareValidatesDataType(t *testing.T) {
	channel := Channel{
		AssetID:  "asset_1",
		Name:     "acceleration_x",
		DataType: "blob",
	}
	if err := channel.Prepare(); err == nil {
		t.Fatal("expected invalid data type error")
	}

	channel.DataType = ""
	if err := channel.Prepare(); err != nil {
		t.Fatalf("expected default data type, got %v", err)
	}
	if channel.DataType != "float64" {
		t.Fatalf("expected float64 data type, got %q", channel.DataType)
	}
}
