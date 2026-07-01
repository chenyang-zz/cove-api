package imagecompress

const (
	defaultMaxEdge     = 1568
	defaultTargetBytes = 3 * 1024 * 1024
)

var defaultQualities = []int{85, 70, 55, 40}

type Options struct {
	MaxEdge     int
	TargetBytes int
	Qualities   []int
}

type Option func(*Options)

func WithMaxEdge(maxEdge int) Option {
	return func(opts *Options) {
		if maxEdge > 0 {
			opts.MaxEdge = maxEdge
		}
	}
}

func WithTargetBytes(targetBytes int) Option {
	return func(opts *Options) {
		if targetBytes > 0 {
			opts.TargetBytes = targetBytes
		}
	}
}

func WithQualities(qualities []int) Option {
	return func(opts *Options) {
		if len(qualities) == 0 {
			return
		}
		out := make([]int, 0, len(qualities))
		for _, quality := range qualities {
			if quality <= 0 || quality > 100 {
				continue
			}
			out = append(out, quality)
		}
		if len(out) != 0 {
			opts.Qualities = out
		}
	}
}
