//go:build !windows

package audio

// Başka sistemlerde henüz yakalama yok (Linux'ta PulseAudio/PipeWire monitor
// kaynağı ayrı bir iş).
func capture(v *Visualizer, ready func(error)) error {
	ready(ErrUnsupported)
	return ErrUnsupported
}
