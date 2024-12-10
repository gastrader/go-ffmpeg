package types

type VideoProcessingConfig struct {
	Outputs     []string
	Resolutions []string
	Bitrates    []string
	AudioRates  []string
	Levels      []string
	Preset      string
	CRF         int
	SegmentTime int
}
