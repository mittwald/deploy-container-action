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

func TestStackActionTestSuite(t *testing.T) {
	suite.Run(t, new(StackActionTestSuite))
}
