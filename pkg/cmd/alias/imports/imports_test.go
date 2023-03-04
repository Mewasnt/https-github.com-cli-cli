package imports

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/internal/config"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/extensions"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/cli/cli/v2/test"
	"github.com/google/shlex"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAliasImports(t *testing.T) {
	tmpFile := filepath.Join(os.TempDir(), "test.yml")
	defer os.Remove(tmpFile)

	tests := []struct {
		name             string
		isTTY            bool
		input            string
		stdin            string
		fileContents     string
		initialConfig    string
		expectedConfig   string
		expectedOutLines []string
		expectedErrLines []string
		wantErr          string
	}{
		{
			name:    "no filename",
			isTTY:   true,
			wantErr: "no filename passed and nothing on STDIN",
		},
		{
			name:    "more than one filename",
			isTTY:   true,
			input:   "aliases1.yml aliases2.yml",
			wantErr: "too many arguments",
		},
		{
			name:  "with no existing aliases",
			isTTY: true,
			input: tmpFile,
			fileContents: heredoc.Doc(`
                co: pr checkout
                igrep: '!gh issue list --label="$1" | grep "$2"'
            `),
			expectedConfig: heredoc.Doc(`
                aliases:
                    co: pr checkout
                    igrep: '!gh issue list --label="$1" | grep "$2"'
            `),
			expectedErrLines: []string{"Importing aliases from file", "Added alias (co|igrep)"},
			expectedOutLines: []string{},
		},
		{
			name:  "with existing aliases",
			isTTY: true,
			input: tmpFile,
			fileContents: heredoc.Doc(`
                users: |-
                    api graphql -F name="$1" -f query='
                        query ($name: String!) {
                            user(login: $name) {
                                name
                            }
                        }'
                co: pr checkout
            `),
			initialConfig: heredoc.Doc(`
                aliases:
                    igrep: '!gh issue list --label="$1" | grep "$2"'
                editor: vim
            `),
			expectedConfig: heredoc.Doc(`
                aliases:
                    igrep: '!gh issue list --label="$1" | grep "$2"'
                    co: pr checkout
                    users: |-
                        api graphql -F name="$1" -f query='
                            query ($name: String!) {
                                user(login: $name) {
                                    name
                                }
                            }'
                editor: vim
            `),
			expectedErrLines: []string{"Importing aliases from file", "Added alias (co|users)"},
			expectedOutLines: []string{},
		},
		{
			name:  "from stdin",
			isTTY: true,
			input: "-",
			stdin: heredoc.Doc(`
                co: pr checkout
                features: |-
                    issue list
                    --label=enhancement
                igrep: '!gh issue list --label="$1" | grep "$2"'
            `),
			expectedConfig: heredoc.Doc(`
                aliases:
                    co: pr checkout
                    features: |-
                        issue list
                        --label=enhancement
                    igrep: '!gh issue list --label="$1" | grep "$2"'
            `),
			expectedErrLines: []string{"Importing aliases from standard input", "Added alias (co|igrep)"},
			expectedOutLines: []string{},
		},
		{
			name:  "already taken aliases",
			isTTY: true,
			input: tmpFile,
			fileContents: heredoc.Doc(`
                co: pr checkout -R cool/repo
                igrep: '!gh issue list --label="$1" | grep "$2"'
            `),
			initialConfig: heredoc.Doc(`
                aliases:
                    co: pr checkout
                editor: vim
            `),
			expectedConfig: heredoc.Doc(`
                aliases:
                    co: pr checkout
                    igrep: '!gh issue list --label="$1" | grep "$2"'
                editor: vim
            `),
			expectedErrLines: []string{
				"Importing aliases from file",
				"Could not import alias co: already taken",
				"Added alias igrep",
			},
			expectedOutLines: []string{},
		},
		{
			name:  "override aliases",
			isTTY: true,
			input: "--clobber " + tmpFile,
			fileContents: heredoc.Doc(`
                co: pr checkout -R cool/repo
                igrep: '!gh issue list --label="$1" | grep "$2"'
            `),
			initialConfig: heredoc.Doc(`
                aliases:
                    co: pr checkout
                editor: vim
            `),
			expectedConfig: heredoc.Doc(`
                aliases:
                    co: pr checkout -R cool/repo
                    igrep: '!gh issue list --label="$1" | grep "$2"'
                editor: vim
            `),
			expectedErrLines: []string{
				"Importing aliases from file",
				"Changed alias co",
				"Added alias igrep",
			},
			expectedOutLines: []string{},
		},
		{
			name:  "alias is a gh command",
			isTTY: true,
			input: tmpFile,
			fileContents: heredoc.Doc(`
                pr: pr checkout
                issue: issue list
                api: api graphql
            `),
			expectedErrLines: []string{"Could not import alias (pr|checkout|list): already a gh command"},
			expectedOutLines: []string{},
		},
		{
			name:  "invalid expansion",
			isTTY: true,
			input: tmpFile,
			fileContents: heredoc.Doc(`
                alias1:
                alias2: ps checkout
            `),
			expectedErrLines: []string{"Could not import alias alias[0-9]: expansion does not correspond to a gh command"},
			expectedOutLines: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.NoError(t, os.WriteFile(tmpFile, []byte(tt.fileContents), 0600))

			readConfigs := config.StubWriteConfig(t)

			cfg := config.NewFromString(tt.initialConfig)

			output, err := runCommand(cfg, tt.isTTY, tt.input, tt.stdin)
			if tt.wantErr != "" {
				assert.EqualError(t, err, tt.wantErr)
			} else {
				require.NoError(t, err)
			}

			mainBuf := bytes.Buffer{}
			readConfigs(&mainBuf, io.Discard)

			//nolint:staticcheck // prefer exact matchers over ExpectLines
			test.ExpectLines(t, output.Stderr(), tt.expectedErrLines...)
			//nolint:staticcheck // prefer exact matchers over ExpectLines
			test.ExpectLines(t, output.String(), tt.expectedOutLines...)

			assert.Equal(t, tt.expectedConfig, mainBuf.String())
		})
	}
}

func runCommand(cfg config.Config, isTTY bool, cli, in string) (*test.CmdOut, error) {
	ios, stdin, stdout, stderr := iostreams.Test()
	ios.SetStdoutTTY(isTTY)
	ios.SetStdinTTY(isTTY)
	ios.SetStderrTTY(isTTY)
	stdin.WriteString(in)

	factory := &cmdutil.Factory{
		IOStreams: ios,
		Config: func() (config.Config, error) {
			return cfg, nil
		},
		ExtensionManager: &extensions.ExtensionManagerMock{
			ListFunc: func() []extensions.Extension {
				return []extensions.Extension{}
			},
		},
	}

	cmd := NewCmdImports(factory, nil)

	// fake command nesting structure needed for validCommand
	rootCmd := &cobra.Command{}
	rootCmd.AddCommand(cmd)
	prCmd := &cobra.Command{Use: "pr"}
	prCmd.AddCommand(&cobra.Command{Use: "checkout"})
	prCmd.AddCommand(&cobra.Command{Use: "status"})
	rootCmd.AddCommand(prCmd)
	issueCmd := &cobra.Command{Use: "issue"}
	issueCmd.AddCommand(&cobra.Command{Use: "list"})
	rootCmd.AddCommand(issueCmd)
	apiCmd := &cobra.Command{Use: "api"}
	apiCmd.AddCommand(&cobra.Command{Use: "graphql"})
	rootCmd.AddCommand(apiCmd)

	argv, err := shlex.Split("imports " + cli)
	if err != nil {
		return nil, err
	}
	rootCmd.SetArgs(argv)
	rootCmd.SetIn(stdin)
	rootCmd.SetOut(io.Discard)
	rootCmd.SetErr(io.Discard)

	err = rootCmd.Execute()
	return &test.CmdOut{
		OutBuf: stdout,
		ErrBuf: stderr,
	}, err
}
