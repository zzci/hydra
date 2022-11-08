package cli

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/ory/x/servicelocatorx"

	"github.com/pkg/errors"

	"github.com/ory/x/configx"

	"github.com/ory/x/errorsx"

	"github.com/ory/x/cmdx"

	"github.com/spf13/cobra"

	"github.com/ory/hydra/driver"
	"github.com/ory/hydra/driver/config"
	"github.com/ory/x/flagx"
)

type MigrateHandler struct{}

func newMigrateHandler() *MigrateHandler {
	return &MigrateHandler{}
}

const (
	genericDialectKey = "any"
)

var fragmentHeader = []byte(strings.TrimLeft(`
-- Migration generated by the command below; DO NOT EDIT.
-- hydra:generate hydra migrate gen
`, "\n"))

var blankFragment = []byte(strings.TrimLeft(`
-- This blank migration was generated to meet ory/x/popx validation criteria, see https://github.com/ory/x/pull/509; DO NOT EDIT.
-- hydra:generate hydra migrate gen
`, "\n"))

var mrx = regexp.MustCompile(`^(\d{14})000000_([^.]+)(\.[a-z0-9]+)?\.(up|down)\.sql$`)

type migration struct {
	Path      string
	ID        string
	Name      string
	Dialect   string
	Direction string
}

type migrationGroup struct {
	ID                    string
	Name                  string
	Children              []*migration
	fallbackUpMigration   *migration
	fallbackDownMigration *migration
}

func (m *migration) ReadSource(fs fs.FS) ([]byte, error) {
	f, err := fs.Open(m.Path)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	defer f.Close()
	return io.ReadAll(f)
}

func (m migration) generateMigrationFragments(source []byte) ([][]byte, error) {
	chunks := bytes.Split(source, []byte("--split"))
	if len(chunks) < 1 {
		return nil, errors.New("no migration chunks found")
	}
	for i := range chunks {
		chunks[i] = append(fragmentHeader, chunks[i]...)
	}
	return chunks, nil
}

func (mg migrationGroup) fragmentName(m *migration, i int) string {
	if m.Dialect == genericDialectKey {
		return fmt.Sprintf("%s%06d_%s.%s.sql", mg.ID, i, mg.Name, m.Direction)
	} else {
		return fmt.Sprintf("%s%06d_%s.%s.%s.sql", mg.ID, i, mg.Name, m.Dialect, m.Direction)
	}
}

// GenerateSQL splits the migration sources into chunks and writes them to the
// target directory.
func (mg migrationGroup) generateSQL(sourceFS fs.FS, target string) error {
	ms := mg.Children
	if mg.fallbackDownMigration != nil {
		ms = append(ms, mg.fallbackDownMigration)
	}
	if mg.fallbackUpMigration != nil {
		ms = append(ms, mg.fallbackUpMigration)
	}
	dialectFragmentCounts := map[string]int{}
	maxFragmentCount := -1
	for _, m := range ms {
		source, err := m.ReadSource(sourceFS)
		if err != nil {
			return errors.WithStack(err)
		}

		fragments, err := m.generateMigrationFragments(source)
		dialectFragmentCounts[m.Dialect] = len(fragments)
		if maxFragmentCount < len(fragments) {
			maxFragmentCount = len(fragments)
		}
		if err != nil {
			return errors.Errorf("failed to process %s: %s", m.Path, err.Error())
		}
		for i, fragment := range fragments {
			dst := filepath.Join(target, mg.fragmentName(m, i))
			if err = os.WriteFile(dst, fragment, 0600); err != nil {
				return errors.WithStack(errors.Errorf("failed to write file %s", dst))
			}
		}
	}
	for _, m := range ms {
		for i := dialectFragmentCounts[m.Dialect]; i < maxFragmentCount; i += 1 {
			dst := filepath.Join(target, mg.fragmentName(m, i))
			if err := os.WriteFile(dst, blankFragment, 0600); err != nil {
				return errors.WithStack(errors.Errorf("failed to write file %s", dst))
			}
		}
	}
	return nil
}

func parseMigration(filename string) (*migration, error) {
	matches := mrx.FindAllStringSubmatch(filename, -1)
	if matches == nil {
		return nil, errors.Errorf("failed to parse migration filename %s; %s does not match pattern ", filename, mrx.String())
	}
	if len(matches) != 1 && len(matches[0]) != 5 {
		return nil, errors.Errorf("invalid migration %s; expected %s", filename, mrx.String())
	}
	dialect := matches[0][3]
	if dialect == "" {
		dialect = genericDialectKey
	} else {
		dialect = dialect[1:]
	}
	return &migration{
		Path:      filename,
		ID:        matches[0][1],
		Name:      matches[0][2],
		Dialect:   dialect,
		Direction: matches[0][4],
	}, nil
}

func readMigrations(migrationSourceFS fs.FS, expectedDialects []string) (map[string]*migrationGroup, error) {
	mgs := make(map[string]*migrationGroup)
	err := fs.WalkDir(migrationSourceFS, ".", func(p string, d fs.DirEntry, err2 error) error {
		if err2 != nil {
			fmt.Println("Warning: unexpected error " + err2.Error())
			return nil
		}
		if d.IsDir() {
			return nil
		}
		if p != filepath.Base(p) {
			fmt.Println("Warning: ignoring nested file " + p)
			return nil
		}

		m, err := parseMigration(p)
		if err != nil {
			return err
		}

		if _, ok := mgs[m.ID]; !ok {
			mgs[m.ID] = &migrationGroup{
				ID:       m.ID,
				Name:     m.Name,
				Children: nil,
			}
		}

		if m.Dialect == genericDialectKey && m.Direction == "up" {
			mgs[m.ID].fallbackUpMigration = m
		} else if m.Dialect == genericDialectKey && m.Direction == "down" {
			mgs[m.ID].fallbackDownMigration = m
		} else {
			mgs[m.ID].Children = append(mgs[m.ID].Children, m)
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	if len(expectedDialects) == 0 {
		return mgs, nil
	}

	eds := make(map[string]struct{})
	for i := range expectedDialects {
		eds[expectedDialects[i]] = struct{}{}
	}
	for _, mg := range mgs {
		expect := make(map[string]struct{})
		for _, m := range mg.Children {
			if _, ok := eds[m.Dialect]; !ok {
				return nil, errors.Errorf("unexpected dialect %s in filename %s", m.Dialect, m.Path)
			}

			expect[m.Dialect+"."+m.Direction] = struct{}{}
		}
		for _, d := range expectedDialects {
			if _, ok := expect[d+".up"]; !ok && mg.fallbackUpMigration == nil {
				return nil, errors.Errorf("dialect %s not found for up migration %s; use --dialects=\"\" to disable dialect validation", d, mg.ID)
			}
			if _, ok := expect[d+".down"]; !ok && mg.fallbackDownMigration == nil {
				return nil, errors.Errorf("dialect %s not found for down migration %s; use --dialects=\"\" to disable dialect validation", d, mg.ID)
			}
		}
	}

	return mgs, nil
}

func (h *MigrateHandler) MigrateGen(cmd *cobra.Command, args []string) {
	cmdx.ExactArgs(cmd, args, 2)
	expectedDialects := flagx.MustGetStringSlice(cmd, "dialects")

	sourceDir := args[0]
	targetDir := args[1]
	sourceFS := os.DirFS(sourceDir)
	mgs, err := readMigrations(sourceFS, expectedDialects)
	if err != nil {
		fmt.Println(err.Error())
		os.Exit(1)
	}
	for _, mg := range mgs {
		err = mg.generateSQL(sourceFS, targetDir)
		if err != nil {
			fmt.Println(err.Error())
			os.Exit(1)
		}
	}

	os.Exit(0)
}

func (h *MigrateHandler) MigrateSQL(cmd *cobra.Command, args []string) (err error) {
	var d driver.Registry

	if flagx.MustGetBool(cmd, "read-from-env") {
		d, err = driver.New(
			cmd.Context(),
			servicelocatorx.NewOptions(),
			[]driver.OptionsModifier{
				driver.WithOptions(
					configx.SkipValidation(),
					configx.WithFlags(cmd.Flags())),
				driver.DisableValidation(),
				driver.DisablePreloading(),
				driver.SkipNetworkInit(),
			})
		if err != nil {
			return err
		}
		if len(d.Config().DSN()) == 0 {
			_, _ = fmt.Fprintln(cmd.ErrOrStderr(), "When using flag -e, environment variable DSN must be set.")
			return cmdx.FailSilently(cmd)
		}
	} else {
		if len(args) != 1 {
			_, _ = fmt.Fprintln(cmd.ErrOrStderr(), "Please provide the database URL.")
			return cmdx.FailSilently(cmd)
		}
		d, err = driver.New(
			cmd.Context(),
			servicelocatorx.NewOptions(),
			[]driver.OptionsModifier{
				driver.WithOptions(
					configx.WithFlags(cmd.Flags()),
					configx.SkipValidation(),
					configx.WithValue(config.KeyDSN, args[0]),
				),
				driver.DisableValidation(),
				driver.DisablePreloading(),
				driver.SkipNetworkInit(),
			})
		if err != nil {
			return err
		}
	}

	p := d.Persister()
	conn := p.Connection(context.Background())
	if conn == nil {
		_, _ = fmt.Fprintln(cmd.ErrOrStderr(), "Migrations can only be executed against a SQL-compatible driver but DSN is not a SQL source.")
		return cmdx.FailSilently(cmd)
	}

	if err := conn.Open(); err != nil {
		_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "Could not open the database connection:\n%+v\n", err)
		return cmdx.FailSilently(cmd)
	}

	// convert migration tables
	if err := p.PrepareMigration(context.Background()); err != nil {
		_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "Could not convert the migration table:\n%+v\n", err)
		return cmdx.FailSilently(cmd)
	}

	// print migration status
	_, _ = fmt.Fprintln(cmd.OutOrStdout(), "The following migration is planned:")

	status, err := p.MigrationStatus(context.Background())
	if err != nil {
		fmt.Fprintf(cmd.ErrOrStderr(), "Could not get the migration status:\n%+v\n", errorsx.WithStack(err))
		return cmdx.FailSilently(cmd)
	}
	_ = status.Write(os.Stdout)

	if !flagx.MustGetBool(cmd, "yes") {
		_, _ = fmt.Fprintln(cmd.ErrOrStderr(), "To skip the next question use flag --yes (at your own risk).")
		if !cmdx.AskForConfirmation("Do you wish to execute this migration plan?", nil, nil) {
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), "Migration aborted.")
			return nil
		}
	}

	// apply migrations
	if err := p.MigrateUp(context.Background()); err != nil {
		_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "Could not apply migrations:\n%+v\n", errorsx.WithStack(err))
		return cmdx.FailSilently(cmd)
	}

	_, _ = fmt.Fprintln(cmd.OutOrStdout(), "Successfully applied migrations!")
	return nil
}