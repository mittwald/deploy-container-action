package main

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"text/template"

	"github.com/mittwald/api-client-go/mittwaldv2/generated/clients/containerclientv2"
	"github.com/mittwald/api-client-go/mittwaldv2/generated/schemas/containerv2"
	"github.com/stretchr/testify/suite"
)

type StackActionTestSuite struct {
	suite.Suite
}

func (s *StackActionTestSuite) SetupTest() {
	os.Clearenv()
}

func (s *StackActionTestSuite) TestRenderConfigTemplate_EnvReplacement() {
	os.Setenv("FOO", "bar")

	tplStr := `value: {{ .Env.FOO }}`
	tpl, err := template.New("test").Parse(tplStr)
	s.Require().NoError(err)

	out, err := renderConfigTemplate(tpl)
	s.Require().NoError(err)

	s.Equal("value: bar", out.String())
}

func (s *StackActionTestSuite) TestLoadYamlOptional_WithTemplatingFromFile() {
	os.Setenv("FOO", "bar")

	content := `key: {{ .Env.FOO }}`
	tmpFile := s.writeTempFile("stack", content)
	os.Setenv("INPUT_STACK_FILE", tmpFile)

	result, err := loadYamlOptional("STACK")
	s.NoError(err)
	s.Equal("bar", result["key"])
}

func (s *StackActionTestSuite) TestLoadYamlOptional_WithTemplatingFromInline() {
	os.Setenv("FOO", "bar")
	os.Setenv("INPUT_STACK_YAML", `key: {{ .Env.FOO }}`)

	result, err := loadYamlOptional("STACK")
	s.NoError(err)
	s.Equal("bar", result["key"])
}

func (s *StackActionTestSuite) TestLoadYamlOptional_FailsWithBadTemplate() {
	os.Setenv("INPUT_STACK_YAML", `key: {{ .Env.`)

	_, err := loadYamlOptional("STACK")
	s.Error(err)
	s.Contains(err.Error(), "template")
}

func (s *StackActionTestSuite) TestMustEnv_Present() {
	os.Setenv("INPUT_API_TOKEN", "dummy-token")
	val := mustEnv("INPUT_API_TOKEN")
	s.Equal("dummy-token", val)
}

func (s *StackActionTestSuite) TestMustEnv_Missing() {
	defer func() {
		if r := recover(); r == nil {
			s.Fail("Expected os.Exit to be called")
		}
	}()
	mustEnv("MISSING_ENV_VAR")
}

func (s *StackActionTestSuite) TestLoadYamlOptional_FromEnv() {
	yamlStr := "key: value"
	os.Setenv("INPUT_STACK_YAML", yamlStr)

	result, err := loadYamlOptional("STACK")
	s.NoError(err)
	s.Equal("value", result["key"])
}

func (s *StackActionTestSuite) TestLoadYamlOptional_Empty() {
	result, err := loadYamlOptional("NON_EXISTENT")
	s.NoError(err)
	s.Nil(result)
}

func (s *StackActionTestSuite) TestLoadYamlRequired_Present() {
	yamlStr := "key: value"
	os.Setenv("INPUT_SERVICES_YAML", yamlStr)

	result, err := loadYamlRequired("SERVICES")
	s.NoError(err)
	s.Equal("value", result["key"])
}

func (s *StackActionTestSuite) TestLoadYamlRequired_Missing() {
	_, err := loadYamlRequired("MISSING")
	s.Error(err)
	s.Contains(err.Error(), "unable to locate inputs")
}

func (s *StackActionTestSuite) TestParseStackObject_Valid() {
	data := map[string]interface{}{
		"services": map[string]interface{}{
			"app": map[string]interface{}{
				"image": "nginx",
			},
		},
	}
	result, err := parseStackObject(data)
	s.NoError(err)
	s.NotNil(result)
	s.Contains(result.Services, "app")
}

func (s *StackActionTestSuite) TestLoadStackData_WithStackYaml() {
	os.Setenv(
		"INPUT_STACK_YAML", `
services:
  app:
    image: nginx
    description: test app
    ports:
      - "80/tcp"
volumes:
  data:
    name: app-volume
`,
	)

	stack, err := loadStackData()
	s.NoError(err)
	s.NotNil(stack)

	stack = addMissingStackData(stack)

	s.Contains(stack.Services, "app")
	s.Equal("nginx", *stack.Services["app"].Image)
	s.Equal("test app", *stack.Services["app"].Description)
	s.Contains(stack.Services["app"].Ports, "80/tcp")
	s.Contains(stack.Volumes, "data")
	s.Equal("app-volume", *stack.Volumes["data"].Name)

	for _, svc := range stack.Services {
		s.NoError(svc.Validate())
	}
	for _, vol := range stack.Volumes {
		s.NoError(vol.Validate())
	}
}

func (s *StackActionTestSuite) TestLoadStackData_WithServicesAndVolumesYaml() {
	os.Setenv(
		"INPUT_SERVICES_YAML", `
app:
  image: nginx
  description: test app
  ports:
    - "80/tcp"
`,
	)
	os.Setenv(
		"INPUT_VOLUMES_YAML", `
data:
  name: app-volume
`,
	)

	stack, err := loadStackData()
	s.NoError(err)
	s.NotNil(stack)

	stack = addMissingStackData(stack)

	s.Contains(stack.Services, "app")
	s.Equal("nginx", *stack.Services["app"].Image)
	s.Equal("test app", *stack.Services["app"].Description)
	s.Contains(stack.Services["app"].Ports, "80/tcp")
	s.Contains(stack.Volumes, "data")
	s.Equal("app-volume", *stack.Volumes["data"].Name)

	for _, svc := range stack.Services {
		s.NoError(svc.Validate())
	}
	for _, vol := range stack.Volumes {
		s.NoError(vol.Validate())
	}
}

func (s *StackActionTestSuite) writeTempFile(prefix, content string) string {
	tmpDir := s.T().TempDir()
	path := filepath.Join(tmpDir, prefix+".yaml")
	err := os.WriteFile(path, []byte(content), 0600)
	s.Require().NoError(err)
	return path
}

func (s *StackActionTestSuite) TestLoadStackData_FromStackFile() {
	content := `
services:
  app:
    image: nginx
    description: test app
    ports:
      - "80/tcp"
volumes:
  data:
    name: app-volume
`
	path := s.writeTempFile("stack", content)
	os.Setenv("INPUT_STACK_FILE", path)

	stack, err := loadStackData()
	s.NoError(err)
	s.NotNil(stack)

	stack = addMissingStackData(stack)

	s.Contains(stack.Services, "app")
	s.Equal("nginx", *stack.Services["app"].Image)
	s.Equal("test app", *stack.Services["app"].Description)
	s.Contains(stack.Services["app"].Ports, "80/tcp")
	s.Contains(stack.Volumes, "data")
	s.Equal("app-volume", *stack.Volumes["data"].Name)

	for _, svc := range stack.Services {
		s.NoError(svc.Validate())
	}
	for _, vol := range stack.Volumes {
		s.NoError(vol.Validate())
	}
}

func (s *StackActionTestSuite) TestLoadStackData_FromSeparateFiles() {
	serviceContent := `
app:
  image: nginx
  description: test app
  ports:
    - "80/tcp"
`
	volumeContent := `
data:
  name: app-volume
`
	servicesPath := s.writeTempFile("services", serviceContent)
	volumesPath := s.writeTempFile("volumes", volumeContent)

	os.Setenv("INPUT_SERVICES_FILE", servicesPath)
	os.Setenv("INPUT_VOLUMES_FILE", volumesPath)

	stack, err := loadStackData()
	s.NoError(err)
	s.NotNil(stack)

	stack = addMissingStackData(stack)

	s.Contains(stack.Services, "app")
	s.Equal("nginx", *stack.Services["app"].Image)
	s.Equal("test app", *stack.Services["app"].Description)
	s.Contains(stack.Services["app"].Ports, "80/tcp")
	s.Contains(stack.Volumes, "data")
	s.Equal("app-volume", *stack.Volumes["data"].Name)

	for _, svc := range stack.Services {
		s.NoError(svc.Validate())
	}
	for _, vol := range stack.Volumes {
		s.NoError(vol.Validate())
	}
}

func (s *StackActionTestSuite) TestLoadStackData_FromInvalidFile() {
	content := `invalid_yaml: [unterminated`
	path := s.writeTempFile("stack", content)
	os.Setenv("INPUT_STACK_FILE", path)

	_, err := loadStackData()
	s.Error(err)
	s.Contains(err.Error(), "unmarshal")
}

func (s *StackActionTestSuite) TestLoadStackData_AllowsMissingServiceDescription() {
	os.Setenv(
		"INPUT_STACK_YAML", `
services:
  app:
    image: nginx
    ports:
      - "80/tcp"
`,
	)

	stack, err := loadStackData()
	s.NoError(err)
	s.NotNil(stack)

	stack = addMissingStackData(stack)

	s.Contains(stack.Services, "app")
	s.NotNil(stack.Services["app"].Description)
	s.Equal("app", *stack.Services["app"].Description)
	for _, svc := range stack.Services {
		s.NoError(svc.Validate())
	}
}

func (s *StackActionTestSuite) TestAddMissingStackData_FillsMissingServiceDescription() {
	stack, err := parseStackObject(map[string]interface{}{
		"services": map[string]interface{}{
			"app": map[string]interface{}{
				"image": "nginx",
			},
		},
	})
	s.NoError(err)
	s.NotNil(stack)

	stack = addMissingStackData(stack)

	s.Contains(stack.Services, "app")
	s.NotNil(stack.Services["app"].Description)
	s.Equal("app", *stack.Services["app"].Description)
	for _, svc := range stack.Services {
		s.NoError(svc.Validate())
	}
}

func (s *StackActionTestSuite) TestAddMissingStackData_EmptyServiceDescription() {
	stack, err := parseStackObject(map[string]interface{}{
		"services": map[string]interface{}{
			"app": map[string]interface{}{
				"image":       "nginx",
				"description": "",
			},
		},
	})
	s.NoError(err)
	s.NotNil(stack)

	stack = addMissingStackData(stack)

	s.Contains(stack.Services, "app")
	s.NotNil(stack.Services["app"].Description)
	s.Equal("app", *stack.Services["app"].Description)
	for _, svc := range stack.Services {
		s.NoError(svc.Validate())
	}
}

func (s *StackActionTestSuite) TestLoadServicesToRecreate_EmptyList() {
	os.Setenv(
		"INPUT_STACK_YAML", `
services:
  app:
    image: nginx
    description: test app
    ports:
      - "80/tcp"
volumes:
  data:
    name: app-volume
`,
	)

	stack, err := loadStackData()
	s.NoError(err)
	s.NotNil(stack)

	stack = addMissingStackData(stack)

	servicesToRecreate := loadServicesToRecreate(stack.Services)
	s.Len(servicesToRecreate, 1)
	_, found := servicesToRecreate["app"]
	s.True(found)
}

func (s *StackActionTestSuite) TestLoadServicesToRecreate_WithOneEntry() {
	os.Setenv(
		"INPUT_STACK_YAML", `
services:
  app:
    image: nginx
    description: test app
    ports:
      - "80/tcp"
  db:
    image: mysql
    description: test mysql
    ports:
      - "3306/tcp"
`,
	)

	os.Setenv("INPUT_SKIP_RECREATION", "db")

	stack, err := loadStackData()
	s.NoError(err)
	s.NotNil(stack)

	stack = addMissingStackData(stack)

	servicesToRecreate := loadServicesToRecreate(stack.Services)
	s.Len(servicesToRecreate, 1)

	_, found := servicesToRecreate["app"]
	s.True(found)
}

func (s *StackActionTestSuite) TestLoadServicesToRecreate_WithMultipleEntries() {
	os.Setenv(
		"INPUT_STACK_YAML", `
services:
  app:
    image: nginx
    description: test app
    ports:
      - "80/tcp"
  db:
    image: mysql
    description: test mysql
    ports:
      - "3306/tcp"
  redis:
    image: redis
    description: test redis
    ports:
      - "6379/tcp"
`,
	)

	os.Setenv("INPUT_SKIP_RECREATION", "db,redis")

	stack, err := loadStackData()
	s.NoError(err)
	s.NotNil(stack)

	stack = addMissingStackData(stack)

	servicesToRecreate := loadServicesToRecreate(stack.Services)
	s.Len(servicesToRecreate, 1)

	_, found := servicesToRecreate["app"]
	s.True(found)
}

func (s *StackActionTestSuite) serviceField(parsed map[string]interface{}, service, field string) interface{} {
	services, ok := parsed["services"].(map[string]interface{})
	s.Require().True(ok, "expected a services map")

	svc, ok := services[service].(map[string]interface{})
	s.Require().True(ok, "expected service %q", service)

	return svc[field]
}

// Regression tests for https://github.com/mittwald/deploy-container-action/issues/151.

func (s *StackActionTestSuite) TestLoadYamlOptional_IgnoresTemplateInFullLineComment() {
	os.Setenv(
		"INPUT_STACK_YAML", `
# db_url: {{ .Env.UNSET_VAR }}
services:
  app:
    image: nginx
`,
	)

	result, err := loadYamlOptional("STACK")
	s.NoError(err)
	s.Equal("nginx", s.serviceField(result, "app", "image"))
}

func (s *StackActionTestSuite) TestLoadYamlOptional_IgnoresTemplateInTrailingComment() {
	os.Setenv(
		"INPUT_STACK_YAML", `
services:
  app:
    image: nginx # was {{ .Env.UNSET_VAR }}
`,
	)

	result, err := loadYamlOptional("STACK")
	s.NoError(err)
	s.Equal("nginx", s.serviceField(result, "app", "image"))
}

func (s *StackActionTestSuite) TestLoadYamlOptional_IgnoresBrokenTemplateSyntaxInComment() {
	os.Setenv(
		"INPUT_STACK_YAML", `
# see {{
services:
  app:
    image: nginx
`,
	)

	result, err := loadYamlOptional("STACK")
	s.NoError(err)
	s.Equal("nginx", s.serviceField(result, "app", "image"))
}

func (s *StackActionTestSuite) TestLoadYamlOptional_KeepsHashInDoubleQuotedScalar() {
	os.Setenv(
		"INPUT_STACK_YAML", `
services:
  app:
    image: nginx
    description: "web # 1" # a real comment
`,
	)

	result, err := loadYamlOptional("STACK")
	s.NoError(err)
	s.Equal("web # 1", s.serviceField(result, "app", "description"))
}

func (s *StackActionTestSuite) TestLoadYamlOptional_KeepsHashInSingleQuotedScalar() {
	os.Setenv(
		"INPUT_STACK_YAML", `
services:
  app:
    image: nginx
    description: 'web # 1'
`,
	)

	result, err := loadYamlOptional("STACK")
	s.NoError(err)
	s.Equal("web # 1", s.serviceField(result, "app", "description"))
}

func (s *StackActionTestSuite) TestLoadYamlOptional_KeepsHashWithoutPrecedingWhitespace() {
	os.Setenv(
		"INPUT_STACK_YAML", `
services:
  app:
    image: nginx#latest
`,
	)

	result, err := loadYamlOptional("STACK")
	s.NoError(err)
	s.Equal("nginx#latest", s.serviceField(result, "app", "image"))
}

func (s *StackActionTestSuite) TestLoadYamlOptional_KeepsShebangInsideBlockScalar() {
	os.Setenv(
		"INPUT_STACK_YAML", `
services:
  app:
    image: nginx
    command: |
      #!/bin/sh
      echo "hello # world"
    description: web # a real comment
`,
	)

	result, err := loadYamlOptional("STACK")
	s.NoError(err)
	s.Equal("#!/bin/sh\necho \"hello # world\"\n", s.serviceField(result, "app", "command"))
	s.Equal("web", s.serviceField(result, "app", "description"))
}

func (s *StackActionTestSuite) TestLoadYamlOptional_StripsCommentOnBlockScalarHeader() {
	os.Setenv(
		"INPUT_STACK_YAML", `
services:
  app:
    image: nginx
    command: | # {{ .Env.UNSET_VAR }}
      #!/bin/sh
`,
	)

	result, err := loadYamlOptional("STACK")
	s.NoError(err)
	s.Equal("#!/bin/sh\n", s.serviceField(result, "app", "command"))
}

func (s *StackActionTestSuite) TestLoadYamlOptional_StillRendersTemplateOutsideComments() {
	os.Setenv("MY_IMAGE", "nginx:1.27")
	os.Setenv(
		"INPUT_STACK_YAML", `
# image: {{ .Env.UNSET_VAR }}
services:
  app:
    image: {{ .Env.MY_IMAGE }}
`,
	)

	result, err := loadYamlOptional("STACK")
	s.NoError(err)
	s.Equal("nginx:1.27", s.serviceField(result, "app", "image"))
}

func (s *StackActionTestSuite) TestLoadYamlOptional_StillFailsOnMissingEnvOutsideComment() {
	os.Setenv(
		"INPUT_STACK_YAML", `
services:
  app:
    image: {{ .Env.UNSET_VAR }}
`,
	)

	_, err := loadYamlOptional("STACK")
	s.Error(err)
	s.Contains(err.Error(), `map has no entry for key "UNSET_VAR"`)
}

func (s *StackActionTestSuite) TestStripYamlComments() {
	testCases := []struct {
		name     string
		input    string
		expected string
	}{
		{"full line comment", "# c {{ .Env.X }}\nkey: v\n", "\nkey: v\n"},
		{"trailing comment", "key: v # {{ .Env.X }}\n", "key: v \n"},
		{"indented comment", "a:\n  # c\n  b: 1\n", "a:\n  \n  b: 1\n"},
		{"hash without whitespace", "key: nginx#latest\n", "key: nginx#latest\n"},
		{"hash in double quotes", "key: \"a # b\" # real\n", "key: \"a # b\" \n"},
		{"hash in single quotes", "key: 'a # b'\n", "key: 'a # b'\n"},
		{"escaped single quote", "key: 'it''s # ok'\n", "key: 'it''s # ok'\n"},
		{
			"block scalar content",
			"cmd: |\n  #!/bin/sh\n  echo hi # keep\nnext: 1 # drop\n",
			"cmd: |\n  #!/bin/sh\n  echo hi # keep\nnext: 1 \n",
		},
		{"folded scalar with chomping", "cmd: >-\n  # keep\nnext: 1 # drop\n", "cmd: >-\n  # keep\nnext: 1 \n"},
		{"comment on block scalar header", "cmd: | # drop\n  # keep\n", "cmd: | \n  # keep\n"},
		{"blank line inside block scalar", "cmd: |\n  a\n\n  b\nnext: 1 # drop\n", "cmd: |\n  a\n\n  b\nnext: 1 \n"},
		{
			"nested block scalar",
			"s:\n  app:\n    cmd: |\n      # keep\n    img: n # drop\n",
			"s:\n  app:\n    cmd: |\n      # keep\n    img: n \n",
		},
		{
			"multiline double quoted scalar",
			"key: \"line one\n  # still string\"\nnext: 1 # drop\n",
			"key: \"line one\n  # still string\"\nnext: 1 \n",
		},
		{"template is untouched", "envs:\n  P: {{ .Env.P }} # note\n", "envs:\n  P: {{ .Env.P }} \n"},
	}

	for _, testCase := range testCases {
		s.Run(
			testCase.name, func() {
				s.Equal(testCase.expected, string(stripYamlComments([]byte(testCase.input))))
			},
		)
	}
}

func (s *StackActionTestSuite) TestStripYamlComments_PreservesLineCount() {
	input := "# one\nkey: v # two\n\n# three\n"
	s.Equal(
		strings.Count(input, "\n"),
		strings.Count(string(stripYamlComments([]byte(input))), "\n"),
	)
}

// updateStackResult is one scripted answer of the fake stack updater.
type updateStackResult struct {
	response     *containerv2.StackResponse
	httpResponse *http.Response
	err          error
}

// fakeStackUpdater replays scripted results in order and repeats the last one afterwards.
type fakeStackUpdater struct {
	results []updateStackResult
	calls   int
	onCall  func()
}

func (f *fakeStackUpdater) UpdateStack(
	_ context.Context,
	_ containerclientv2.UpdateStackRequest,
	_ ...func(req *http.Request) error,
) (*containerv2.StackResponse, *http.Response, error) {
	index := f.calls
	if index >= len(f.results) {
		index = len(f.results) - 1
	}
	f.calls++

	if f.onCall != nil {
		f.onCall()
	}

	result := f.results[index]

	return result.response, result.httpResponse, result.err
}

func httpResponseWithStatus(status int) *http.Response {
	return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(""))}
}

var (
	errDialTimeout        = errors.New("dial tcp: i/o timeout")
	errServiceUnavailable = errors.New("503")
	errTooManyRequests    = errors.New("429")
	errBadRequest         = errors.New("400")
	errBadGateway         = errors.New("502")
	errAboveHTTPRange     = errors.New("600")
	errBuildRequest       = errors.New("failed to marshal request body")
)

func transportError() error {
	return &url.Error{Op: "Patch", URL: "https://api.mittwald.de/v2/stacks/x", Err: errDialTimeout}
}

func (s *StackActionTestSuite) TestUpdateStackWithRetry_SucceedsAfterTransportErrors() {
	success := &containerv2.StackResponse{}
	fake := &fakeStackUpdater{results: []updateStackResult{
		{err: transportError()},
		{err: transportError()},
		{response: success, httpResponse: httpResponseWithStatus(http.StatusOK)},
	}}

	response, _, err := updateStackWithRetry(context.Background(), fake, containerclientv2.UpdateStackRequest{}, 0)

	s.Require().NoError(err)
	s.Same(success, response)
	s.Equal(3, fake.calls)
}

func (s *StackActionTestSuite) TestUpdateStackWithRetry_RetriesServerErrors() {
	fake := &fakeStackUpdater{results: []updateStackResult{
		{httpResponse: httpResponseWithStatus(http.StatusServiceUnavailable), err: errServiceUnavailable},
		{httpResponse: httpResponseWithStatus(http.StatusTooManyRequests), err: errTooManyRequests},
		{response: &containerv2.StackResponse{}, httpResponse: httpResponseWithStatus(http.StatusOK)},
	}}

	_, _, err := updateStackWithRetry(context.Background(), fake, containerclientv2.UpdateStackRequest{}, 0)

	s.Require().NoError(err)
	s.Equal(3, fake.calls)
}

func (s *StackActionTestSuite) TestUpdateStackWithRetry_DoesNotRetryClientErrors() {
	fake := &fakeStackUpdater{results: []updateStackResult{
		{httpResponse: httpResponseWithStatus(http.StatusBadRequest), err: errBadRequest},
	}}

	_, httpResponse, err := updateStackWithRetry(context.Background(), fake, containerclientv2.UpdateStackRequest{}, 0)

	s.Require().ErrorIs(err, errBadRequest)
	s.Equal(http.StatusBadRequest, httpResponse.StatusCode) // response is kept for diagnostics
	s.Equal(1, fake.calls)
}

func (s *StackActionTestSuite) TestUpdateStackWithRetry_DoesNotRetryRequestBuildErrors() {
	fake := &fakeStackUpdater{results: []updateStackResult{{err: errBuildRequest}}}

	_, httpResponse, err := updateStackWithRetry(context.Background(), fake, containerclientv2.UpdateStackRequest{}, 0)

	s.Require().ErrorIs(err, errBuildRequest)
	s.Nil(httpResponse)
	s.Equal(1, fake.calls)
}

func (s *StackActionTestSuite) TestUpdateStackWithRetry_GivesUpAfterMaxAttempts() {
	fake := &fakeStackUpdater{results: []updateStackResult{{err: transportError()}}}

	_, httpResponse, err := updateStackWithRetry(context.Background(), fake, containerclientv2.UpdateStackRequest{}, 0)

	s.Require().Error(err)
	s.Nil(httpResponse) // nothing to dump — main must not dereference it
	s.Equal(updateStackMaxAttempts, fake.calls)
}

func (s *StackActionTestSuite) TestUpdateStackWithRetry_StopsWhenContextIsCancelled() {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	fake := &fakeStackUpdater{results: []updateStackResult{{err: transportError()}}}

	_, httpResponse, err := updateStackWithRetry(ctx, fake, containerclientv2.UpdateStackRequest{}, 0)

	s.Require().ErrorIs(err, context.Canceled)
	s.Nil(httpResponse)
	s.Equal(0, fake.calls)
}

func (s *StackActionTestSuite) TestUpdateStackWithRetry_ReportsCancellationDuringBackoff() {
	ctx, cancel := context.WithCancel(context.Background())
	fake := &fakeStackUpdater{results: []updateStackResult{{err: transportError()}}}
	fake.onCall = cancel // cancelled while the first attempt is in flight

	_, httpResponse, err := updateStackWithRetry(ctx, fake, containerclientv2.UpdateStackRequest{}, 0)

	s.Require().ErrorIs(err, context.Canceled)
	s.Contains(err.Error(), "i/o timeout") // the transient failure is kept as cause
	s.Nil(httpResponse)                    // its body was closed, nothing usable to return
	s.Equal(1, fake.calls)
}

func (s *StackActionTestSuite) TestIsTransientFailure_IgnoresStatusCodesAboveTheHTTPRange() {
	s.True(isTransientFailure(httpResponseWithStatus(http.StatusBadGateway), errBadGateway))
	s.False(isTransientFailure(httpResponseWithStatus(600), errAboveHTTPRange))
}

func TestStackActionTestSuite(t *testing.T) {
	suite.Run(t, new(StackActionTestSuite))
}
