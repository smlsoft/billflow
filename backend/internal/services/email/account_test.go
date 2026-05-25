package emailservice

import (
	"errors"
	"testing"
)

func TestShouldDrainBacklog(t *testing.T) {
	tests := []struct {
		name string
		res  PollResult
		want bool
	}{
		{
			name: "limited result continues in background",
			res:  PollResult{Limited: true, Backlog: 100},
			want: true,
		},
		{
			name: "backlog result continues in background",
			res:  PollResult{Backlog: 1},
			want: true,
		},
		{
			name: "clean result stops",
			res:  PollResult{},
			want: false,
		},
		{
			name: "hard error stops",
			res:  PollResult{Err: errors.New("imap failed"), Limited: true, Backlog: 100},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := shouldDrainBacklog(tt.res); got != tt.want {
				t.Fatalf("shouldDrainBacklog() = %v, want %v", got, tt.want)
			}
		})
	}
}
