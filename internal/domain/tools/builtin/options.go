package builtin

import "time"

type options struct {
	clock func() time.Time
}

// Option 修改领域工具目录的长期配置。
type Option func(*options)

func defaultOptions() options {
	return options{
		clock: time.Now,
	}
}

// WithClock 设置 current_time 等时间相关工具使用的时钟。
//
// clock 为 nil 时忽略该配置，继续使用默认的 time.Now。该选项主要用于测试中
// 注入固定时间，避免用例受运行时刻影响。
func WithClock(clock func() time.Time) Option {
	return func(opts *options) {
		if clock != nil {
			opts.clock = clock
		}
	}
}

func applyOptions(opts ...Option) options {
	out := defaultOptions()
	for _, opt := range opts {
		if opt != nil {
			opt(&out)
		}
	}
	return out
}
