package jetbrains

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// MCPConfig represents the standard JetBrains .ai/mcp/mcp.json structure.
type MCPConfig struct {
	MCPServers map[string]MCPServerEntry `json:"mcpServers"`
}

// MCPServerEntry represents a single MCP server configuration.
type MCPServerEntry struct {
	Command string            `json:"command"`
	Args    []string          `json:"args"`
	Env     map[string]string `json:"env,omitempty"`
}

// SetupOptions controls what gets configured for JetBrains.
type SetupOptions struct {
	ProjectDir             string
	BinaryPath             string
	ConfigureProjectMCP    bool
	ConfigureGlobalMCP     bool
	ConfigureRunConfigs    bool
	ConfigureExternalTools bool
	GlobalAIPathOverride   string // Optional override for tests
	JetBrainsHomeOverride  string // Optional override for tests
}

// SetupResult captures the files generated or updated.
type SetupResult struct {
	FilesCreated []string `json:"files_created"`
	FilesUpdated []string `json:"files_updated"`
	ToolsAdded   []string `json:"tools_added"`
}

// SetupJetBrainsToolset configures JetBrains IDEs to natively recognize Dendrite MCP and autonomous agents.
func SetupJetBrainsToolset(opts SetupOptions) (*SetupResult, error) {
	if opts.ProjectDir == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return nil, fmt.Errorf("failed to determine project directory: %w", err)
		}
		opts.ProjectDir = cwd
	}

	absProjectDir, err := filepath.Abs(opts.ProjectDir)
	if err != nil {
		return nil, fmt.Errorf("invalid project directory path: %w", err)
	}

	if opts.BinaryPath == "" {
		opts.BinaryPath = filepath.Join(absProjectDir, "bin", "dendrite")
	}

	res := &SetupResult{
		FilesCreated: []string{},
		FilesUpdated: []string{},
		ToolsAdded:   []string{},
	}

	// 1. Project-level MCP configuration (.ai/mcp/mcp.json)
	if opts.ConfigureProjectMCP {
		projectMCPPath := filepath.Join(absProjectDir, ".ai", "mcp", "mcp.json")
		if err := configureMCPFile(projectMCPPath, opts.BinaryPath, absProjectDir, res); err != nil {
			return nil, fmt.Errorf("failed to configure project MCP: %w", err)
		}
	}

	// 2. Global / Application-level MCP configuration (~/.ai/mcp/mcp.json)
	if opts.ConfigureGlobalMCP {
		var globalMCPPath string
		if opts.GlobalAIPathOverride != "" {
			globalMCPPath = opts.GlobalAIPathOverride
		} else {
			home, err := os.UserHomeDir()
			if err != nil {
				return nil, fmt.Errorf("failed to determine user home directory: %w", err)
			}
			globalMCPPath = filepath.Join(home, ".ai", "mcp", "mcp.json")
		}
		if err := configureMCPFile(globalMCPPath, opts.BinaryPath, absProjectDir, res); err != nil {
			return nil, fmt.Errorf("failed to configure global MCP: %w", err)
		}
	}

	// 3. JetBrains Run Configurations (.idea/runConfigurations/)
	if opts.ConfigureRunConfigs {
		runConfigDir := filepath.Join(absProjectDir, ".idea", "runConfigurations")
		if err := os.MkdirAll(runConfigDir, 0755); err != nil {
			return nil, fmt.Errorf("failed to create runConfigurations dir: %w", err)
		}

		configs := getRunConfigurationTemplates()
		for name, xmlContent := range configs {
			path := filepath.Join(runConfigDir, name)
			existed := fileExists(path)
			if err := os.WriteFile(path, []byte(strings.TrimSpace(xmlContent)+"\n"), 0644); err != nil {
				return nil, fmt.Errorf("failed to write run configuration %s: %w", name, err)
			}
			if existed {
				res.FilesUpdated = append(res.FilesUpdated, path)
			} else {
				res.FilesCreated = append(res.FilesCreated, path)
			}
		}
	}

	// 4. JetBrains External Tools (.idea/tools/External_Tools.xml)
	if opts.ConfigureExternalTools {
		toolsDir := filepath.Join(absProjectDir, ".idea", "tools")
		if err := os.MkdirAll(toolsDir, 0755); err != nil {
			return nil, fmt.Errorf("failed to create tools dir: %w", err)
		}
		toolsPath := filepath.Join(toolsDir, "External_Tools.xml")
		existed := fileExists(toolsPath)
		xmlContent := getExternalToolsXML()
		if err := os.WriteFile(toolsPath, []byte(strings.TrimSpace(xmlContent)+"\n"), 0644); err != nil {
			return nil, fmt.Errorf("failed to write external tools file: %w", err)
		}
		if existed {
			res.FilesUpdated = append(res.FilesUpdated, toolsPath)
		} else {
			res.FilesCreated = append(res.FilesCreated, toolsPath)
		}
		res.ToolsAdded = append(res.ToolsAdded,
			"Dendrite: Run Local Agent Triage on Active File",
			"Dendrite: Run Aider on Active File",
			"Dendrite: Run Antigravity on Active File",
			"Dendrite: Probe MCP Diagnostics",
			"Dendrite: List Registered MCP Tools",
			"Dendrite: Telemetry Snapshot",
		)
	}

	return res, nil
}

func configureMCPFile(targetPath, binaryPath, projectDir string, res *SetupResult) error {
	dir := filepath.Dir(targetPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	config := MCPConfig{
		MCPServers: make(map[string]MCPServerEntry),
	}

	existed := false
	if data, err := os.ReadFile(targetPath); err == nil && len(strings.TrimSpace(string(data))) > 0 {
		existed = true
		_ = json.Unmarshal(data, &config)
		if config.MCPServers == nil {
			config.MCPServers = make(map[string]MCPServerEntry)
		}
	}

	config.MCPServers["dendrite"] = MCPServerEntry{
		Command: binaryPath,
		Args:    []string{"mcp", "serve"},
		Env: map[string]string{
			"DENDRITE_WORKSPACE":  projectDir,
			"DENDRITE_ENV":        "development",
			"DENDRITE_OLLAMA_URL": "http://127.0.0.1:11434",
		},
	}

	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return err
	}

	if err := os.WriteFile(targetPath, append(data, '\n'), 0644); err != nil {
		return err
	}

	if existed {
		res.FilesUpdated = append(res.FilesUpdated, targetPath)
	} else {
		res.FilesCreated = append(res.FilesCreated, targetPath)
	}
	return nil
}

func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

func getRunConfigurationTemplates() map[string]string {
	return map[string]string{
		"Dendrite_Local_Agent_Triage.xml": `<component name="ProjectRunConfigurationManager">
  <configuration default="false" name="Dendrite: Local Agent Triage" type="GoApplicationRunConfiguration" factoryName="Go Application">
    <module name="Dendrite" />
    <working_directory value="$PROJECT_DIR$" />
    <parameters value="triage --prompt &quot;Evaluate task complexity, select skills, and optimize token spend&quot;" />
    <kind value="PACKAGE" />
    <package value="Dendrite/cmd/dendrite" />
    <directory value="$PROJECT_DIR$" />
    <filePath value="$PROJECT_DIR$/cmd/dendrite/main.go" />
    <method v="2" />
  </configuration>
</component>`,
		"Dendrite_MCP_Server.xml": `<component name="ProjectRunConfigurationManager">
  <configuration default="false" name="Dendrite: MCP Server" type="GoApplicationRunConfiguration" factoryName="Go Application">
    <module name="Dendrite" />
    <working_directory value="$PROJECT_DIR$" />
    <parameters value="mcp serve" />
    <kind value="PACKAGE" />
    <package value="Dendrite/cmd/dendrite" />
    <directory value="$PROJECT_DIR$" />
    <filePath value="$PROJECT_DIR$/cmd/dendrite/main.go" />
    <method v="2" />
  </configuration>
</component>`,
		"Dendrite_Agent_Aider.xml": `<component name="ProjectRunConfigurationManager">
  <configuration default="false" name="Dendrite: Agent Runner (Aider)" type="GoApplicationRunConfiguration" factoryName="Go Application">
    <module name="Dendrite" />
    <working_directory value="$PROJECT_DIR$" />
    <parameters value="agent run aider --ticket DEN-050 --task &quot;Autonomous implementation loop&quot;" />
    <kind value="PACKAGE" />
    <package value="Dendrite/cmd/dendrite" />
    <directory value="$PROJECT_DIR$" />
    <filePath value="$PROJECT_DIR$/cmd/dendrite/main.go" />
    <method v="2" />
  </configuration>
</component>`,
		"Dendrite_Agent_Antigravity.xml": `<component name="ProjectRunConfigurationManager">
  <configuration default="false" name="Dendrite: Agent Runner (Antigravity)" type="GoApplicationRunConfiguration" factoryName="Go Application">
    <module name="Dendrite" />
    <working_directory value="$PROJECT_DIR$" />
    <parameters value="agent run agy --task &quot;Strategic architecture synthesis&quot;" />
    <kind value="PACKAGE" />
    <package value="Dendrite/cmd/dendrite" />
    <directory value="$PROJECT_DIR$" />
    <filePath value="$PROJECT_DIR$/cmd/dendrite/main.go" />
    <method v="2" />
  </configuration>
</component>`,
		"Dendrite_Web_UI.xml": `<component name="ProjectRunConfigurationManager">
  <configuration default="false" name="Dendrite: Web UI &amp; Dashboard" type="GoApplicationRunConfiguration" factoryName="Go Application">
    <module name="Dendrite" />
    <working_directory value="$PROJECT_DIR$" />
    <parameters value="ui --port 3456" />
    <kind value="PACKAGE" />
    <package value="Dendrite/cmd/dendrite" />
    <directory value="$PROJECT_DIR$" />
    <filePath value="$PROJECT_DIR$/cmd/dendrite/main.go" />
    <method v="2" />
  </configuration>
</component>`,
		"Dendrite_Doctor.xml": `<component name="ProjectRunConfigurationManager">
  <configuration default="false" name="Dendrite: Pre-flight Doctor" type="GoApplicationRunConfiguration" factoryName="Go Application">
    <module name="Dendrite" />
    <working_directory value="$PROJECT_DIR$" />
    <parameters value="doctor" />
    <kind value="PACKAGE" />
    <package value="Dendrite/cmd/dendrite" />
    <directory value="$PROJECT_DIR$" />
    <filePath value="$PROJECT_DIR$/cmd/dendrite/main.go" />
    <method v="2" />
  </configuration>
</component>`,
		"Dendrite_JetBrains_Probe.xml": `<component name="ProjectRunConfigurationManager">
  <configuration default="false" name="Dendrite: Probe JetBrains MCP" type="GoApplicationRunConfiguration" factoryName="Go Application">
    <module name="Dendrite" />
    <working_directory value="$PROJECT_DIR$" />
    <parameters value="jetbrains probe" />
    <kind value="PACKAGE" />
    <package value="Dendrite/cmd/dendrite" />
    <directory value="$PROJECT_DIR$" />
    <filePath value="$PROJECT_DIR$/cmd/dendrite/main.go" />
    <method v="2" />
  </configuration>
</component>`,
	}
}

func getExternalToolsXML() string {
	return `<toolSet name="External Tools">
  <tool name="Dendrite: Run Local Agent Triage on Active File" showInMainMenu="true" showInEditor="true" showInProject="true" showInSearchPopup="true" disabled="false" useConsole="true" showConsoleOnStdOut="true" showConsoleOnStdErr="true" synchronizeAfterRun="false">
    <exec>
      <option name="COMMAND" value="$ProjectFileDir$/bin/dendrite" />
      <option name="PARAMETERS" value="triage --file $FilePath$ --prompt &quot;Optimize context and triage task&quot;" />
      <option name="WORKING_DIRECTORY" value="$ProjectFileDir$" />
    </exec>
  </tool>
  <tool name="Dendrite: Run Aider on Active File" showInMainMenu="true" showInEditor="true" showInProject="true" showInSearchPopup="true" disabled="false" useConsole="true" showConsoleOnStdOut="true" showConsoleOnStdErr="true" synchronizeAfterRun="true">
    <exec>
      <option name="COMMAND" value="$ProjectFileDir$/bin/dendrite" />
      <option name="PARAMETERS" value="agent run aider --task &quot;Refactor and improve $FileName$&quot;" />
      <option name="WORKING_DIRECTORY" value="$ProjectFileDir$" />
    </exec>
  </tool>
  <tool name="Dendrite: Run Antigravity on Active File" showInMainMenu="true" showInEditor="true" showInProject="true" showInSearchPopup="true" disabled="false" useConsole="true" showConsoleOnStdOut="true" showConsoleOnStdErr="true" synchronizeAfterRun="true">
    <exec>
      <option name="COMMAND" value="$ProjectFileDir$/bin/dendrite" />
      <option name="PARAMETERS" value="agent run agy --task &quot;Analyze architecture and specs for $FilePath$&quot;" />
      <option name="WORKING_DIRECTORY" value="$ProjectFileDir$" />
    </exec>
  </tool>
  <tool name="Dendrite: Probe MCP Diagnostics" showInMainMenu="true" showInEditor="true" showInProject="true" showInSearchPopup="true" disabled="false" useConsole="true" showConsoleOnStdOut="true" showConsoleOnStdErr="true" synchronizeAfterRun="true">
    <exec>
      <option name="COMMAND" value="$ProjectFileDir$/bin/dendrite" />
      <option name="PARAMETERS" value="jetbrains problems $FilePath$" />
      <option name="WORKING_DIRECTORY" value="$ProjectFileDir$" />
    </exec>
  </tool>
  <tool name="Dendrite: List Registered MCP Tools" showInMainMenu="true" showInEditor="true" showInProject="true" showInSearchPopup="true" disabled="false" useConsole="true" showConsoleOnStdOut="true" showConsoleOnStdErr="true" synchronizeAfterRun="false">
    <exec>
      <option name="COMMAND" value="$ProjectFileDir$/bin/dendrite" />
      <option name="PARAMETERS" value="mcp tools" />
      <option name="WORKING_DIRECTORY" value="$ProjectFileDir$" />
    </exec>
  </tool>
  <tool name="Dendrite: Telemetry Snapshot" showInMainMenu="true" showInEditor="true" showInProject="true" showInSearchPopup="true" disabled="false" useConsole="true" showConsoleOnStdOut="true" showConsoleOnStdErr="true" synchronizeAfterRun="false">
    <exec>
      <option name="COMMAND" value="$ProjectFileDir$/bin/dendrite" />
      <option name="PARAMETERS" value="telemetry" />
      <option name="WORKING_DIRECTORY" value="$ProjectFileDir$" />
    </exec>
  </tool>
</toolSet>`
}
