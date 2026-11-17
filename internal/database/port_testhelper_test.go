package database

import "testing"

type fakePort struct {
	value string
}

func (p fakePort) Port() string {
	return p.value
}

func TestMappedPortInt(t *testing.T) {
	tests := []struct {
		name    string
		port    string
		want    int
		wantErr bool
	}{
		{name: "valid numeric port", port: "5432", want: 5432},
		{name: "malformed port", port: "not-a-port", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := mappedPortInt(fakePort{value: tt.port})
			if (err != nil) != tt.wantErr {
				t.Fatalf("mappedPortInt() error = %v, wantErr %v", err, tt.wantErr)
			}
			if got != tt.want {
				t.Fatalf("mappedPortInt() = %d, want %d", got, tt.want)
			}
		})
	}
}
