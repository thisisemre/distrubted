package config

import (
	"bufio"
	"os"
	"regexp"
	"strconv"
	"strings"

	"github.com/rotr/ring-of-the-middle-earth/internal/game"
)

// AppConfig holds all loaded configuration.
type AppConfig struct {
	Units      map[string]game.UnitConfig
	Regions    map[string]game.RegionConfig
	Paths      map[string]game.PathConfig
	GameConfig game.GameConfig
}

// Load reads units.conf and map.conf from the given config directory.
func Load(configDir string) (*AppConfig, error) {
	cfg := &AppConfig{
		Units:   make(map[string]game.UnitConfig),
		Regions: make(map[string]game.RegionConfig),
		Paths:   make(map[string]game.PathConfig),
		GameConfig: game.GameConfig{
			HiddenUntilTurn:     3,
			MaxTurns:            40,
			TurnDurationSeconds: 60,
		},
	}

	if err := loadUnits(configDir+"/units.conf", cfg); err != nil {
		return nil, err
	}
	if err := loadMap(configDir+"/map.conf", cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}

// loadUnits parses the HOCON-like units.conf file.
func loadUnits(path string, cfg *AppConfig) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	content := string(data)

	// Extract global settings
	cfg.GameConfig.HiddenUntilTurn = extractInt(content, "hidden-until-turn", 3)
	cfg.GameConfig.MaxTurns = extractInt(content, "max-turns", 40)
	cfg.GameConfig.TurnDurationSeconds = extractInt(content, "turn-duration-seconds", 60)

	// Parse unit blocks: each { ... } block
	units := parseUnitBlocks(content)
	for _, u := range units {
		cfg.Units[u.ID] = u
	}
	return nil
}

// loadMap parses map.conf for regions and paths.
func loadMap(path string, cfg *AppConfig) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	content := string(data)

	regions := parseRegionBlocks(content)
	for _, r := range regions {
		cfg.Regions[r.ID] = r
	}

	paths := parsePathBlocks(content)
	for _, p := range paths {
		cfg.Paths[p.ID] = p
	}
	return nil
}

func extractInt(content, key string, defaultVal int) int {
	re := regexp.MustCompile(key + `\s*=\s*(\d+)`)
	m := re.FindStringSubmatch(content)
	if len(m) < 2 {
		return defaultVal
	}
	v, err := strconv.Atoi(m[1])
	if err != nil {
		return defaultVal
	}
	return v
}

func parseUnitBlocks(content string) []game.UnitConfig {
	var units []game.UnitConfig
	// Find content between units = [ and closing ]
	start := strings.Index(content, "units = [")
	if start == -1 {
		return units
	}
	// Find the closing bracket at same level
	depth := 0
	inUnits := false
	blockStart := -1
	for i := start; i < len(content); i++ {
		c := content[i]
		if c == '[' {
			depth++
			if depth == 1 {
				inUnits = true
			}
		} else if c == ']' {
			depth--
			if depth == 0 && inUnits {
				break
			}
		} else if c == '{' && depth == 1 {
			blockStart = i
		} else if c == '}' && depth == 1 && blockStart >= 0 {
			block := content[blockStart : i+1]
			u := parseUnitBlock(block)
			units = append(units, u)
			blockStart = -1
		}
	}
	return units
}

func parseUnitBlock(block string) game.UnitConfig {
	u := game.UnitConfig{}
	u.ID = extractString(block, "id")
	u.Name = extractString(block, "name")
	u.Class = game.UnitClass(extractString(block, "class"))
	u.Side = game.Side(extractString(block, "side"))
	u.StartRegion = extractString(block, "start")
	u.Strength = extractInt(block, "strength", 1)
	u.Leadership = extractBool(block, "leadership")
	u.LeadershipBonus = extractInt(block, "leadershipBonus", 0)
	u.Indestructible = extractBool(block, "indestructible")
	u.DetectionRange = extractInt(block, "detectionRange", 0)
	u.Respawns = extractBool(block, "respawns")
	u.RespawnTurns = extractInt(block, "respawnTurns", 0)
	u.Maia = extractBool(block, "maia")
	u.MaiaAbilityPaths = extractStringArray(block, "maiaAbilityPaths")
	u.IgnoresFortress = extractBool(block, "ignoresFortress")
	u.CanFortify = extractBool(block, "canFortify")
	u.Cooldown = extractInt(block, "cooldown", 0)
	return u
}

func parseRegionBlocks(content string) []game.RegionConfig {
	var regions []game.RegionConfig
	start := strings.Index(content, "regions = [")
	if start == -1 {
		return regions
	}
	depth := 0
	blockStart := -1
	for i := start; i < len(content); i++ {
		c := content[i]
		if c == '[' {
			depth++
		} else if c == ']' {
			depth--
			if depth == 0 {
				break
			}
		} else if c == '{' && depth == 1 {
			blockStart = i
		} else if c == '}' && depth == 1 && blockStart >= 0 {
			block := content[blockStart : i+1]
			r := parseRegionBlock(block)
			regions = append(regions, r)
			blockStart = -1
		}
	}
	return regions
}

func parseRegionBlock(block string) game.RegionConfig {
	return game.RegionConfig{
		ID:           extractString(block, "id"),
		Name:         extractString(block, "name"),
		Terrain:      game.Terrain(extractString(block, "terrain")),
		SpecialRole:  game.SpecialRole(extractString(block, "specialRole")),
		StartControl: game.Side(extractString(block, "startControl")),
		StartThreat:  extractInt(block, "startThreat", 0),
	}
}

func parsePathBlocks(content string) []game.PathConfig {
	var paths []game.PathConfig
	start := strings.Index(content, "paths = [")
	if start == -1 {
		return paths
	}
	depth := 0
	blockStart := -1
	for i := start; i < len(content); i++ {
		c := content[i]
		if c == '[' {
			depth++
		} else if c == ']' {
			depth--
			if depth == 0 {
				break
			}
		} else if c == '{' && depth == 1 {
			blockStart = i
		} else if c == '}' && depth == 1 && blockStart >= 0 {
			block := content[blockStart : i+1]
			p := parsePathBlock(block)
			paths = append(paths, p)
			blockStart = -1
		}
	}
	return paths
}

func parsePathBlock(block string) game.PathConfig {
	return game.PathConfig{
		ID:   extractString(block, "id"),
		From: extractString(block, "from"),
		To:   extractString(block, "to"),
		Cost: extractInt(block, "cost", 1),
	}
}

func extractString(block, key string) string {
	re := regexp.MustCompile(`(?:^|\s)` + regexp.QuoteMeta(key) + `\s*=\s*"([^"]*)"`)
	m := re.FindStringSubmatch(block)
	if len(m) >= 2 {
		return m[1]
	}
	return ""
}

func extractBool(block, key string) bool {
	re := regexp.MustCompile(`(?:^|\s)` + regexp.QuoteMeta(key) + `\s*=\s*(true|false)`)
	m := re.FindStringSubmatch(block)
	if len(m) >= 2 {
		return m[1] == "true"
	}
	return false
}

func extractStringArray(block, key string) []string {
	re := regexp.MustCompile(`(?:^|\s)` + regexp.QuoteMeta(key) + `\s*=\s*\[([^\]]*)\]`)
	m := re.FindStringSubmatch(block)
	if len(m) < 2 {
		return []string{}
	}
	inner := m[1]
	if strings.TrimSpace(inner) == "" {
		return []string{}
	}
	var result []string
	scanner := bufio.NewScanner(strings.NewReader(inner))
	scanner.Split(bufio.ScanWords)
	itemRe := regexp.MustCompile(`"([^"]+)"`)
	for scanner.Scan() {
		word := scanner.Text()
		sub := itemRe.FindAllStringSubmatch(word, -1)
		for _, s := range sub {
			if len(s) >= 2 {
				result = append(result, s[1])
			}
		}
	}
	// Also handle comma-separated inline
	if len(result) == 0 {
		parts := strings.Split(inner, ",")
		for _, p := range parts {
			p = strings.TrimSpace(p)
			p = strings.Trim(p, `"`)
			if p != "" {
				result = append(result, p)
			}
		}
	}
	return result
}
