package main

import (
	"os"
	"path/filepath"
	"testing"
	"text/template"

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

// assertRejectsLineBreak sets a single environment variable and expects the given template to be
// rejected because that variable's value would break out of its position in the YAML document.
func (s *StackActionTestSuite) assertRejectsLineBreak(envName, envValue, stackYaml string) {
	s.T().Helper()

	os.Setenv(envName, envValue)
	os.Setenv("INPUT_STACK_YAML", stackYaml)

	result, err := loadYamlOptional("STACK")
	s.Require().Error(err)
	s.Nil(result)
	s.Contains(err.Error(), envName)
	s.Contains(err.Error(), "line break")
}

func (s *StackActionTestSuite) TestLoadYamlOptional_RejectsLineBreakInEnvValue() {
	injection := "nginx\nvolumes:\n  evil:\n    name: pwned"

	testCases := map[string]struct {
		envValue  string
		stackYaml string
	}{
		"value position": {
			envValue:  injection,
			stackYaml: "services:\n  app:\n    image: {{ .Env.INJECTED }}\n",
		},
		"comment position": {
			envValue:  "line1\nimage: evil",
			stackYaml: "services:\n  app:\n    image: nginx\n# db: {{ .Env.INJECTED }}\n",
		},
		"quoted value position": {
			envValue:  "nginx\"\nvolumes:\n  evil:\n    name: \"pwned",
			stackYaml: "services:\n  app:\n    image: \"{{ .Env.INJECTED }}\"\n",
		},
		"carriage return line feed": {
			envValue:  "nginx\r\nvolumes:\r\n  evil:\r\n    name: pwned",
			stackYaml: "services:\n  app:\n    image: {{ .Env.INJECTED }}\n",
		},
		"lone carriage return": {
			envValue:  "nginx\rvolumes:\r  evil:\r    name: pwned",
			stackYaml: "services:\n  app:\n    image: {{ .Env.INJECTED }}\n",
		},
		"trailing carriage return": {
			envValue:  "nginx\r",
			stackYaml: "services:\n  app:\n    image: {{ .Env.INJECTED }}\n",
		},
		"multi line secret in quoted value": {
			envValue:  "-----BEGIN RSA PRIVATE KEY-----\nMIIBOgIBAAJBAK7\n-----END RSA PRIVATE KEY-----",
			stackYaml: "services:\n  app:\n    image: nginx\n    envs:\n      KEY: \"{{ .Env.INJECTED }}\"\n",
		},
		"index access": {
			envValue:  injection,
			stackYaml: "services:\n  app:\n    image: {{ index .Env \"INJECTED\" }}\n",
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

func (s *StackActionTestSuite) TestLoadYamlOptional_IgnoresUnreferencedLineBreakEnvValue() {
	os.Setenv("GITHUB_EVENT", "{\n  \"action\": \"opened\"\n}")
	os.Setenv("INPUT_STACK_YAML", "services:\n  app:\n    image: nginx\n")

	result, err := loadYamlOptional("STACK")
	s.Require().NoError(err)

	services, ok := result["services"].(map[string]interface{})
	s.Require().True(ok)
	s.Contains(services, "app")
}

func (s *StackActionTestSuite) TestLoadYamlOptional_SubstitutesWhileUnreferencedLineBreakEnvValueIsPresent() {
	os.Setenv("GITHUB_EVENT", "{\n  \"action\": \"opened\"\n}")
	os.Setenv("IMAGE", "nginx:1.27")
	os.Setenv("INPUT_STACK_YAML", "services:\n  app:\n    image: {{ .Env.IMAGE }}\n")

	result, err := loadYamlOptional("STACK")
	s.Require().NoError(err)

	services, ok := result["services"].(map[string]interface{})
	s.Require().True(ok)
	app, ok := services["app"].(map[string]interface{})
	s.Require().True(ok)
	s.Equal("nginx:1.27", app["image"])
}

func (s *StackActionTestSuite) TestLoadYamlOptional_AllowsLineBreakEnvValueInUntakenBranch() {
	os.Setenv("MODE", "off")
	os.Setenv("PRIVATE_KEY", "-----BEGIN RSA PRIVATE KEY-----\nMIIBOgIBAAJBAK7\n-----END RSA PRIVATE KEY-----")
	os.Setenv(
		"INPUT_STACK_YAML",
		"services:\n  app:\n    image: {{ if eq .Env.MODE \"on\" }}{{ .Env.PRIVATE_KEY }}{{ else }}nginx{{ end }}\n",
	)

	result, err := loadYamlOptional("STACK")
	s.Require().NoError(err)

	services, ok := result["services"].(map[string]interface{})
	s.Require().True(ok)
	app, ok := services["app"].(map[string]interface{})
	s.Require().True(ok)
	s.Equal("nginx", app["image"])
}

func TestStackActionTestSuite(t *testing.T) {
	suite.Run(t, new(StackActionTestSuite))
}
