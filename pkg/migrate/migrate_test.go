package migrate

import (
	"testing"
	"testing/fstest"
)

func TestLoadOrdenaMigracionesPorVersion(t *testing.T) {
	migrations, err := load(fstest.MapFS{
		"002_second.up.sql": {Data: []byte("SELECT 2;")},
		"001_first.up.sql":  {Data: []byte("SELECT 1;")},
	})
	if err != nil {
		t.Fatalf("cargar migraciones: %v", err)
	}
	if len(migrations) != 2 || migrations[0].version != 1 || migrations[1].version != 2 {
		t.Fatalf("orden inesperado: %+v", migrations)
	}
}

func TestLoadRechazaNombreOContenidoInvalido(t *testing.T) {
	cases := []fstest.MapFS{
		{"migration.up.sql": {Data: []byte("SELECT 1;")}},
		{"001_empty.up.sql": {Data: []byte("  \n")}},
	}
	for _, source := range cases {
		if _, err := load(source); err == nil {
			t.Fatal("una migracion invalida debe devolver error")
		}
	}
}
