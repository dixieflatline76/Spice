package wallpaper

// TuningConfig holds the internal magic numbers and thresholds for image processing.
// These are currently static but centralized here to allow for future remote configuration.
type TuningConfig struct {
	// Smart Fit 2.0 (Core)
	AspectThreshold      float64 `json:"aspect_threshold"`      // Default: 0.9 (Base tolerance)
	AggressiveMultiplier float64 `json:"aggressive_multiplier"` // Default: 1.9 (Flexibility mode boost)

	// Dual Safety (v1.6.2)
	MinEnergyThreshold           float64 `json:"min_energy_threshold"`             // Default: 0.05
	FeetGuardRatio               float64 `json:"feet_guard_ratio"`                 // Default: 0.5 (Legacy/Simple check)
	FeetGuardSlackThreshold      float64 `json:"feet_guard_slack_threshold"`       // Default: 0.8 (Slack-aware check)
	FeetGuardSlackRelaxed        float64 `json:"feet_guard_slack_relaxed"`         // Default: 1.00 (Fully relaxed for high energy)
	FeetGuardHighEnergyThreshold float64 `json:"feet_guard_high_energy_threshold"` // Default: 0.2 (Trust SmartCrop more)
	EnergyThumbSize              int     `json:"energy_thumb_size"`                // Default: 128 (Performance/Accuracy balance)

	// Face Logic (v1.6.1+)
	FaceRescueQThreshold    float32 `json:"face_rescue_q_threshold"`    // Default: 20.0
	FaceBottomEdgeThreshold float64 `json:"face_bottom_edge_threshold"` // Default: 0.7 (Bottom 30% danger zone)
	FaceBottomEdgeMinQ      float32 `json:"face_bottom_edge_min_q"`     // Default: 20.0 (Req higher Q for edges)
	FaceIoUThreshold        float64 `json:"face_iou_threshold"`         // Default: 0.2 (Clustering)
	FaceScaleFactor         float64 `json:"face_scale_factor"`          // Default: 1.1 (pigo internal)
	FaceDetectConfidence    float64 `json:"face_detect_confidence"`     // Default: 10.0 (Base filter)
	FaceDetectMinSizePct    int     `json:"face_detect_min_size_pct"`   // Default: 1 (1% of min dim)
	FaceDetectShift         float64 `json:"face_detect_shift"`          // Default: 0.1 (Stride)

	// Encoding
	EncodingQuality int `json:"encoding_quality"` // Default: 95
}

// DefaultTuningConfig returns the standard values for Spice 1.6.2.
func DefaultTuningConfig() TuningConfig {
	return TuningConfig{
		AspectThreshold:              0.9,
		AggressiveMultiplier:         2.5,
		MinEnergyThreshold:           0.05,
		FeetGuardRatio:               0.5,
		FeetGuardSlackThreshold:      0.8,
		FeetGuardSlackRelaxed:        1.0,
		FeetGuardHighEnergyThreshold: 0.2,
		EnergyThumbSize:              128,
		FaceRescueQThreshold:         20.0,
		FaceBottomEdgeThreshold:      0.7,
		FaceBottomEdgeMinQ:           20.0,
		FaceIoUThreshold:             0.2,
		FaceScaleFactor:              1.1,
		FaceDetectConfidence:         10.0,
		FaceDetectMinSizePct:         1,
		FaceDetectShift:              0.1,
		EncodingQuality:              95,
	}
}
