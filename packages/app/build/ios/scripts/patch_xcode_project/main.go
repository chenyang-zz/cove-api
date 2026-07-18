// Command patch_xcode_project applies the iOS build settings that Wails' alpha
// Xcode template currently omits for a Go c-archive application.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

const (
	objcLinkerFlag          = `OTHER_LDFLAGS = "-ObjC";`
	fullBleedBuildMark      = "full_bleed_overlay.go"
	defaultLaunchBackground = "<color key=\"backgroundColor\" white=\"1\" alpha=\"1\" colorSpace=\"custom\" customColorSpace=\"genericGamma22GrayColorSpace\"/>"
	dynamicLaunchBackground = "<color key=\"backgroundColor\" name=\"LaunchBackground\"/>"
	fullBleedBuild          = `env -u GOOS -u GOARCH -u CGO_ENABLED -u CGO_CFLAGS -u CGO_LDFLAGS \"%[1]s\" run build/ios/scripts/full_bleed_overlay.go -overlay build/ios/xcode/overlay.json -modfile build/ios/xcode/fullbleed.mod -sumfile build/ios/xcode/fullbleed.sum\n\"%[1]s\" build -p=1 -mod=mod -modfile build/ios/xcode/fullbleed.mod -buildmode=c-archive -overlay build/ios/xcode/overlay.json -o \"bin/$1.a\"`
)

var (
	bundleNamePattern       = regexp.MustCompile(`(?s)<key>CFBundleName</key>\s*<string>([^<]+)</string>`)
	bundleExecutablePattern = regexp.MustCompile(`(?s)(<key>CFBundleExecutable</key>\s*<string>)[^<]*(</string>)`)
	archiveBuildPattern     = regexp.MustCompile(`go build -buildmode=c-archive -overlay build/ios/xcode/overlay\.json -o \\"bin/([^\\"]+)\.a\\"`)
)

func main() {
	projectPath := flag.String("project", "", "path to project.pbxproj")
	plistPath := flag.String("plist", "", "path to the generated Info.plist")
	launchStoryboardPath := flag.String("launch-storyboard", "", "path to the generated launch storyboard")
	assetCatalogPath := flag.String("asset-catalog", "", "path to the generated asset catalog")
	flag.Parse()
	if *projectPath == "" || *plistPath == "" || *launchStoryboardPath == "" || *assetCatalogPath == "" {
		fatalf("-project, -plist, -launch-storyboard and -asset-catalog are required")
	}
	goExecutable, err := exec.LookPath("go")
	if err != nil {
		fatalf("locate Go executable: %v", err)
	}

	patchProject(*projectPath, goExecutable)
	patchPlist(*plistPath)
	patchLaunchScreen(*launchStoryboardPath)
	writeLaunchBackgroundAsset(*assetCatalogPath)
}

func patchProject(path, goExecutable string) {
	contents, err := os.ReadFile(path)
	if err != nil {
		fatalf("read project: %v", err)
	}
	project := string(contents)
	if !strings.Contains(project, objcLinkerFlag) {
		if !strings.Contains(project, "CODE_SIGNING_ALLOWED = NO;") {
			fatalf("unexpected Xcode signing template")
		}
		project = strings.ReplaceAll(project, "CODE_SIGNING_ALLOWED = NO;", "CODE_SIGNING_ALLOWED = YES;\n\t\t\t\t"+objcLinkerFlag)
	}
	fullBleedScript := fmt.Sprintf(fullBleedBuild, goExecutable)
	if !strings.Contains(project, fullBleedBuildMark) {
		if !archiveBuildPattern.MatchString(project) {
			fatalf("unexpected Xcode archive build script")
		}
		project = archiveBuildPattern.ReplaceAllString(project, fullBleedScript)
	} else if strings.Contains(project, "full_bleed_overlay.go") {
		if strings.Contains(project, goExecutable) {
			project = strings.ReplaceAll(project, "\""+goExecutable+"\"", "\\\""+goExecutable+"\\\"")
		} else {
			project = strings.ReplaceAll(project, "env -u GOOS -u GOARCH -u CGO_ENABLED -u CGO_CFLAGS -u CGO_LDFLAGS go run build/ios/scripts/full_bleed_overlay.go", fmt.Sprintf(`env -u GOOS -u GOARCH -u CGO_ENABLED -u CGO_CFLAGS -u CGO_LDFLAGS \"%s\" run build/ios/scripts/full_bleed_overlay.go`, goExecutable))
			project = strings.ReplaceAll(project, "\\ngo build -p=1 -mod=mod -modfile build/ios/xcode/fullbleed.mod", fmt.Sprintf(`\n\"%s\" build -p=1 -mod=mod -modfile build/ios/xcode/fullbleed.mod`, goExecutable))
		}
	}
	if err := os.WriteFile(path, []byte(project), 0o644); err != nil {
		fatalf("write project: %v", err)
	}
}

func patchPlist(path string) {
	contents, err := os.ReadFile(path)
	if err != nil {
		fatalf("read Info.plist: %v", err)
	}
	plist := string(contents)
	nameMatch := bundleNamePattern.FindStringSubmatch(plist)
	if len(nameMatch) != 2 || nameMatch[1] == "" {
		fatalf("could not determine product name from Info.plist")
	}
	if !bundleExecutablePattern.MatchString(plist) {
		fatalf("Info.plist is missing CFBundleExecutable")
	}
	plist = bundleExecutablePattern.ReplaceAllString(plist, "${1}"+nameMatch[1]+"${2}")
	if err := os.WriteFile(path, []byte(plist), 0o644); err != nil {
		fatalf("write Info.plist: %v", err)
	}
}

func patchLaunchScreen(path string) {
	contents, err := os.ReadFile(path)
	if err != nil {
		fatalf("read launch storyboard: %v", err)
	}
	storyboard := string(contents)
	if !strings.Contains(storyboard, dynamicLaunchBackground) {
		if !strings.Contains(storyboard, defaultLaunchBackground) {
			fatalf("unexpected launch storyboard background")
		}
		storyboard = strings.Replace(storyboard, defaultLaunchBackground, dynamicLaunchBackground, 1)
	}
	if err := os.WriteFile(path, []byte(storyboard), 0o644); err != nil {
		fatalf("write launch storyboard: %v", err)
	}
}

func writeLaunchBackgroundAsset(catalogPath string) {
	assetDirectory := filepath.Join(catalogPath, "LaunchBackground.colorset")
	if err := os.MkdirAll(assetDirectory, 0o755); err != nil {
		fatalf("create launch colour asset: %v", err)
	}

	colour := func(red, green, blue string, dark bool) map[string]any {
		entry := map[string]any{
			"idiom": "universal",
			"color": map[string]any{
				"color-space": "srgb",
				"components": map[string]string{
					"alpha": "1.000",
					"red":   red,
					"green": green,
					"blue":  blue,
				},
			},
		}
		if dark {
			entry["appearances"] = []map[string]string{{"appearance": "luminosity", "value": "dark"}}
		}
		return entry
	}

	asset := map[string]any{
		"colors": []map[string]any{
			colour("0.945", "0.973", "0.973", false),
			colour("0.043", "0.125", "0.153", true),
		},
		"info": map[string]string{"author": "xcode", "version": "1"},
	}
	contents, err := json.MarshalIndent(asset, "", "  ")
	if err != nil {
		fatalf("encode launch colour asset: %v", err)
	}
	if err := os.WriteFile(filepath.Join(assetDirectory, "Contents.json"), append(contents, '\n'), 0o644); err != nil {
		fatalf("write launch colour asset: %v", err)
	}
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "patch iOS Xcode project: "+format+"\n", args...)
	os.Exit(1)
}
