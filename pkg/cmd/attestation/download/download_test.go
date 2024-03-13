package download

import (
	"bufio"
	"bytes"
	"fmt"
	"net/http"
	"os"
	"path"
	"testing"

	"github.com/cli/cli/v2/pkg/cmd/attestation/api"
	"github.com/cli/cli/v2/pkg/cmd/attestation/artifact"
	"github.com/cli/cli/v2/pkg/cmd/attestation/artifact/oci"
	"github.com/cli/cli/v2/pkg/cmd/attestation/logging"
	"github.com/cli/cli/v2/pkg/cmdutil"

	"github.com/cli/cli/v2/pkg/httpmock"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/google/shlex"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewDownloadCmd(t *testing.T) {
	testIO, _, _, _ := iostreams.Test()
	f := &cmdutil.Factory{
		IOStreams: testIO,
		HttpClient: func() (*http.Client, error) {
			reg := &httpmock.Registry{}
			client := &http.Client{}
			httpmock.ReplaceTripper(client, reg)
			return client, nil
		},
	}
	tempDir := t.TempDir()

	testcases := []struct {
		name     string
		cli      string
		wants    Options
		wantsErr bool
	}{
		{
			name: "Invalid digest-alg flag",
			cli:  "../test/data/sigstore-js-2.1.0.tgz --owner sigstore --digest-alg sha384",
			wants: Options{
				ArtifactPath:    "../test/data/sigstore-js-2.1.0.tgz",
				APIClient:       api.NewTestClient(),
				OCIClient:       oci.MockClient{},
				DigestAlgorithm: "sha384",
				Owner:           "sigstore",
				OutputPath:      tempDir,
				Limit:           30,
			},
			wantsErr: true,
		},
		{
			name: "Missing digest-alg flag",
			cli:  "../test/data/sigstore-js-2.1.0.tgz --owner sigstore",
			wants: Options{
				ArtifactPath:    "../test/data/sigstore-js-2.1.0.tgz",
				APIClient:       api.NewTestClient(),
				OCIClient:       oci.MockClient{},
				DigestAlgorithm: "sha256",
				Owner:           "sigstore",
				OutputPath:      tempDir,
				Limit:           30,
			},
			wantsErr: false,
		},
		{
			name: "Missing owner and repo flags",
			cli:  "../test/data/sigstore-js-2.1.0.tgz",
			wants: Options{
				ArtifactPath:    "../test/data/sigstore-js-2.1.0.tgz",
				APIClient:       api.NewTestClient(),
				OCIClient:       oci.MockClient{},
				DigestAlgorithm: "sha256",
				Owner:           "sigstore",
				OutputPath:      tempDir,
				Limit:           30,
			},
			wantsErr: true,
		},
		{
			name: "Has both owner and repo flags",
			cli:  "../test/data/sigstore-js-2.1.0.tgz --owner sigstore --repo sigstore/sigstore-js",
			wants: Options{
				ArtifactPath:    "../test/data/sigstore-js-2.1.0.tgz",
				APIClient:       api.NewTestClient(),
				OCIClient:       oci.MockClient{},
				DigestAlgorithm: "sha256",
				Owner:           "sigstore",
				OutputPath:      tempDir,
				Repo:            "sigstore/sigstore-js",
				Limit:           30,
			},
			wantsErr: true,
		},
		{
			name: "Uses default limit flag",
			cli:  "../test/data/sigstore-js-2.1.0.tgz --owner sigstore",
			wants: Options{
				ArtifactPath:    "../test/data/sigstore-js-2.1.0.tgz",
				APIClient:       api.NewTestClient(),
				OCIClient:       oci.MockClient{},
				DigestAlgorithm: "sha256",
				Owner:           "sigstore",
				OutputPath:      tempDir,
				Limit:           30,
			},
			wantsErr: false,
		},
		{
			name: "Uses custom limit flag",
			cli:  "../test/data/sigstore-js-2.1.0.tgz --owner sigstore --limit 101",
			wants: Options{
				ArtifactPath:    "../test/data/sigstore-js-2.1.0.tgz",
				APIClient:       api.NewTestClient(),
				OCIClient:       oci.MockClient{},
				DigestAlgorithm: "sha256",
				Owner:           "sigstore",
				OutputPath:      tempDir,
				Limit:           101,
			},
			wantsErr: false,
		},
		{
			name: "Uses invalid limit flag",
			cli:  "../test/data/sigstore-js-2.1.0.tgz --owner sigstore --limit 0",
			wants: Options{
				ArtifactPath:    "../test/data/sigstore-js-2.1.0.tgz",
				APIClient:       api.NewTestClient(),
				OCIClient:       oci.MockClient{},
				DigestAlgorithm: "sha256",
				Owner:           "sigstore",
				OutputPath:      tempDir,
				Limit:           0,
			},
			wantsErr: true,
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			var opts *Options
			cmd := NewDownloadCmd(f, func(o *Options) error {
				opts = o
				return nil
			})

			argv, err := shlex.Split(tc.cli)
			assert.NoError(t, err)
			cmd.SetArgs(argv)
			cmd.SetIn(&bytes.Buffer{})
			cmd.SetOut(&bytes.Buffer{})
			cmd.SetErr(&bytes.Buffer{})
			_, err = cmd.ExecuteC()
			if tc.wantsErr {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)

			assert.Equal(t, tc.wants.DigestAlgorithm, opts.DigestAlgorithm)
			assert.Equal(t, tc.wants.Limit, opts.Limit)
			assert.Equal(t, tc.wants.Owner, opts.Owner)
			assert.Equal(t, tc.wants.Repo, opts.Repo)
		})
	}
}

func TestRunDownload(t *testing.T) {
	tempDir := t.TempDir()

	baseOpts := Options{
		ArtifactPath:    "../test/data/sigstore-js-2.1.0.tgz",
		APIClient:       api.NewTestClient(),
		OCIClient:       oci.MockClient{},
		DigestAlgorithm: "sha512",
		Owner:           "sigstore",
		OutputPath:      tempDir,
		Limit:           30,
		Logger:          logging.NewTestLogger(),
	}

	t.Run("fetch and store attestations successfully with owner", func(t *testing.T) {
		err := runDownload(&baseOpts)
		require.NoError(t, err)

		artifact, err := artifact.NewDigestedArtifact(baseOpts.OCIClient, baseOpts.ArtifactPath, baseOpts.DigestAlgorithm)
		require.NoError(t, err)

		require.FileExists(t, fmt.Sprintf("%s/%s.jsonl", tempDir, artifact.DigestWithAlg()))

		actualLineCount, err := countLines(fmt.Sprintf("%s/%s.jsonl", tempDir, artifact.DigestWithAlg()))
		require.NoError(t, err)

		expectedLineCount := 2
		require.Equal(t, expectedLineCount, actualLineCount)
	})

	t.Run("fetch and store attestations successfully with repo", func(t *testing.T) {
		opts := baseOpts
		opts.Owner = ""
		opts.Repo = "sigstore/sigstore-js"

		err := runDownload(&opts)
		require.NoError(t, err)

		artifact, err := artifact.NewDigestedArtifact(opts.OCIClient, opts.ArtifactPath, opts.DigestAlgorithm)
		require.NoError(t, err)

		require.FileExists(t, fmt.Sprintf("%s/%s.jsonl", tempDir, artifact.DigestWithAlg()))

		actualLineCount, err := countLines(fmt.Sprintf("%s/%s.jsonl", tempDir, artifact.DigestWithAlg()))
		require.NoError(t, err)

		expectedLineCount := 2
		require.Equal(t, expectedLineCount, actualLineCount)
	})

	t.Run("download OCI image attestations successfully", func(t *testing.T) {
		opts := baseOpts
		opts.ArtifactPath = "oci://ghcr.io/github/test"

		err := runDownload(&opts)
		require.NoError(t, err)

		artifact, err := artifact.NewDigestedArtifact(opts.OCIClient, opts.ArtifactPath, opts.DigestAlgorithm)
		require.NoError(t, err)

		require.FileExists(t, fmt.Sprintf("%s/%s.jsonl", tempDir, artifact.DigestWithAlg()))

		actualLineCount, err := countLines(fmt.Sprintf("%s/%s.jsonl", tempDir, artifact.DigestWithAlg()))
		require.NoError(t, err)

		expectedLineCount := 2
		require.Equal(t, expectedLineCount, actualLineCount)
	})

	t.Run("cannot find artifact", func(t *testing.T) {
		opts := baseOpts
		opts.ArtifactPath = "../test/data/not-real.zip"

		err := runDownload(&opts)
		require.Error(t, err)
	})

	t.Run("no attestations found", func(t *testing.T) {
		opts := baseOpts
		opts.APIClient = api.MockClient{
			OnGetByOwnerAndDigest: func(repo, digest string, limit int) ([]*api.Attestation, error) {
				return nil, nil
			},
		}

		err := runDownload(&opts)
		require.NoError(t, err)

		artifact, err := artifact.NewDigestedArtifact(opts.OCIClient, opts.ArtifactPath, opts.DigestAlgorithm)
		require.NoError(t, err)
		require.NoFileExists(t, artifact.DigestWithAlg())
	})

	t.Run("cannot download OCI artifact", func(t *testing.T) {
		opts := baseOpts
		opts.ArtifactPath = "oci://ghcr.io/github/test"
		opts.OCIClient = oci.ReferenceFailClient{}

		err := runDownload(&opts)
		require.Error(t, err)
		require.ErrorContains(t, err, "failed to digest artifact")
	})

	t.Run("with missing OCI client", func(t *testing.T) {
		customOpts := baseOpts
		customOpts.ArtifactPath = "oci://ghcr.io/github/test"
		customOpts.OCIClient = nil
		require.Error(t, runDownload(&customOpts))
	})

	t.Run("with missing API client", func(t *testing.T) {
		customOpts := baseOpts
		customOpts.APIClient = nil
		require.Error(t, runDownload(&customOpts))
	})
}

func TestCreateJSONLinesFilePath(t *testing.T) {
	tempDir := t.TempDir()
	artifact, err := artifact.NewDigestedArtifact(oci.MockClient{}, "../test/data/sigstore-js-2.1.0.tgz", "sha512")
	require.NoError(t, err)

	outputFileName := fmt.Sprintf("%s.jsonl", artifact.DigestWithAlg())

	testCases := []struct {
		name       string
		outputPath string
		expected   string
	}{
		{
			name:       "with output path",
			outputPath: tempDir,
			expected:   path.Join(tempDir, outputFileName),
		},
		{
			name:       "with nested output path",
			outputPath: path.Join(tempDir, "subdir"),
			expected:   path.Join(tempDir, "subdir", outputFileName),
		},
		{
			name:       "with output path with beginning slash",
			outputPath: path.Join("/", tempDir, "subdir"),
			expected:   path.Join("/", tempDir, "subdir", outputFileName),
		},
		{
			name:       "without output path",
			outputPath: "",
			expected:   outputFileName,
		},
	}

	for _, tc := range testCases {
		actualPath := createJSONLinesFilePath(artifact.DigestWithAlg(), tc.outputPath)
		require.Equal(t, tc.expected, actualPath)
	}
}

func countLines(path string) (int, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer f.Close()

	counter := 0
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		counter += 1
	}

	return counter, nil
}
