package genx

import "encoding/json"

// SetTemperature configures temperature, including an explicit zero. Existing
// zero-valued Go struct literals continue to leave temperature unspecified.
func (p *ModelParams) SetTemperature(value float32) {
	p.Temperature = value
	p.temperatureSet = true
}

// HasTemperature reports whether a temperature was explicitly configured.
// Positive values in existing Go struct literals also count as configured.
func (p *ModelParams) HasTemperature() bool {
	return p != nil && (p.temperatureSet || p.Temperature > 0)
}

// MarshalJSON preserves an explicit zero while omitting an unspecified one.
func (p ModelParams) MarshalJSON() ([]byte, error) {
	type plain ModelParams
	if !p.temperatureSet || p.Temperature != 0 {
		return json.Marshal(plain(p))
	}
	return json.Marshal(struct {
		plain
		Temperature float32 `json:"temperature"`
	}{plain: plain(p), Temperature: p.Temperature})
}

// UnmarshalJSON retains temperature presence. Missing and null fields preserve
// an existing value, matching encoding/json's behavior for numeric fields.
func (p *ModelParams) UnmarshalJSON(data []byte) error {
	type plain ModelParams
	value := plain(*p)
	wire := struct {
		*plain
		Temperature json.RawMessage `json:"temperature"`
	}{plain: &value}
	if err := json.Unmarshal(data, &wire); err != nil {
		return err
	}
	if len(wire.Temperature) != 0 {
		var temperature *float32
		if err := json.Unmarshal(wire.Temperature, &temperature); err != nil {
			return err
		}
		if temperature != nil {
			value.Temperature = *temperature
			value.temperatureSet = true
		}
	}
	*p = ModelParams(value)
	return nil
}
