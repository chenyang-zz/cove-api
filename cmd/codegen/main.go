package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"sort"
)

var ErrCheckFailed = errors.New("codegen check failed: generated files are out of date")

func main() {
	if len(os.Args) < 2 {
		printUsage(os.Stderr)
		os.Exit(2)
	}

	var err error
	switch os.Args[1] {
	case "route":
		_, err = runRouteCommand(os.Args[2:])
	case "repository":
		_, err = runRepositoryCommand(os.Args[2:])
	case "docs":
		_, err = runDocsCommand(os.Args[2:])
	case "prompt":
		_, err = runPromptCommand(os.Args[2:])
	case "doctor":
		_, err = runDoctorCommand(os.Args[2:])
	case "help", "-h", "--help":
		printUsage(os.Stdout)
		return
	default:
		printUsage(os.Stderr)
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func runRouteCommand(args []string) (Report, error) {
	fs := flag.NewFlagSet("route", flag.ContinueOnError)
	root := fs.String("root", ".", "project root")
	dryRun := fs.Bool("dry-run", false, "preview generated files without writing")
	check := fs.Bool("check", false, "fail if generated files are out of date")
	verbose := fs.Bool("verbose", false, "print scan diagnostics")
	list := fs.Bool("list", false, "list route generation targets")
	format := fs.String("format", "text", "output format: text or json")
	noColor := fs.Bool("no-color", false, "disable colored output")
	if err := fs.Parse(args); err != nil {
		return Report{}, err
	}
	if *dryRun && *check {
		return Report{}, fmt.Errorf("--dry-run and --check are mutually exclusive")
	}
	reportFormat := ReportFormat(*format)

	if *list {
		items, err := ListRoutes(*root)
		if err != nil {
			return Report{}, err
		}
		return Report{Root: *root, Command: "route list", Mode: ModeWrite}, printRouteList(os.Stdout, items, reportFormat, !*noColor)
	}

	report, err := GenerateRoutesWithOptions(RouteOptions{
		Root:    *root,
		DryRun:  *dryRun,
		Check:   *check,
		Verbose: *verbose,
	})
	if err != nil {
		return report, err
	}
	if err := printReportWithFormat(os.Stdout, report, reportFormat, !*noColor); err != nil {
		return report, err
	}
	if *check && report.Changed() {
		return report, ErrCheckFailed
	}
	return report, nil
}

func runRepositoryCommand(args []string) (Report, error) {
	fs := flag.NewFlagSet("repository", flag.ContinueOnError)
	root := fs.String("root", ".", "project root")
	model := fs.String("model", "", "GORM model name")
	label := fs.String("label", "", "human readable model label for errors")
	scope := fs.String("scope", "", "repository user scope, format local_column:table.column:user_column")
	dryRun := fs.Bool("dry-run", false, "preview generated files without writing")
	check := fs.Bool("check", false, "fail if generated files are out of date")
	verbose := fs.Bool("verbose", false, "print scan diagnostics")
	listModels := fs.Bool("list-models", false, "list GORM models and repository scope status")
	format := fs.String("format", "text", "output format: text or json")
	noColor := fs.Bool("no-color", false, "disable colored output")
	if err := fs.Parse(args); err != nil {
		return Report{}, err
	}
	if *dryRun && *check {
		return Report{}, fmt.Errorf("--dry-run and --check are mutually exclusive")
	}
	reportFormat := ReportFormat(*format)

	if *listModels {
		items, err := ListRepositoryModels(*root)
		if err != nil {
			return Report{}, err
		}
		return Report{Root: *root, Command: "repository list-models", Mode: ModeWrite}, printRepositoryModelList(os.Stdout, items, reportFormat, !*noColor)
	}

	report, err := GenerateRepository(RepositoryOptions{
		Root:    *root,
		Model:   *model,
		Label:   *label,
		Scope:   *scope,
		DryRun:  *dryRun,
		Check:   *check,
		Verbose: *verbose,
	})
	if err != nil {
		return report, err
	}
	if err := printReportWithFormat(os.Stdout, report, reportFormat, !*noColor); err != nil {
		return report, err
	}
	if *check && report.Changed() {
		return report, ErrCheckFailed
	}
	return report, nil
}

func runDocsCommand(args []string) (Report, error) {
	fs := flag.NewFlagSet("docs", flag.ContinueOnError)
	root := fs.String("root", ".", "project root")
	output := fs.String("output", defaultDocsOutput, "OpenAPI JSON output path")
	title := fs.String("title", "Cove API", "OpenAPI info.title")
	version := fs.String("version", "0.1.0", "OpenAPI info.version")
	dryRun := fs.Bool("dry-run", false, "preview generated files without writing")
	check := fs.Bool("check", false, "fail if generated files are out of date")
	verbose := fs.Bool("verbose", false, "print scan diagnostics")
	format := fs.String("format", "text", "output format: text or json")
	noColor := fs.Bool("no-color", false, "disable colored output")
	if err := fs.Parse(args); err != nil {
		return Report{}, err
	}
	if *dryRun && *check {
		return Report{}, fmt.Errorf("--dry-run and --check are mutually exclusive")
	}
	report, err := GenerateDocs(DocsOptions{
		Root:    *root,
		Output:  *output,
		Title:   *title,
		Version: *version,
		DryRun:  *dryRun,
		Check:   *check,
		Verbose: *verbose,
	})
	if err != nil {
		return report, err
	}
	if err := printReportWithFormat(os.Stdout, report, ReportFormat(*format), !*noColor); err != nil {
		return report, err
	}
	if *check && report.Changed() {
		return report, ErrCheckFailed
	}
	return report, nil
}

func runDoctorCommand(args []string) (Report, error) {
	fs := flag.NewFlagSet("doctor", flag.ContinueOnError)
	root := fs.String("root", ".", "project root")
	verbose := fs.Bool("verbose", false, "print scan diagnostics")
	format := fs.String("format", "text", "output format: text or json")
	noColor := fs.Bool("no-color", false, "disable colored output")
	if err := fs.Parse(args); err != nil {
		return Report{}, err
	}
	report, err := RunDoctor(DoctorOptions{Root: *root, Verbose: *verbose})
	if err != nil {
		return report, err
	}
	if err := printReportWithFormat(os.Stdout, report, ReportFormat(*format), !*noColor); err != nil {
		return report, err
	}
	return report, nil
}

func runPromptCommand(args []string) (Report, error) {
	fs := flag.NewFlagSet("prompt", flag.ContinueOnError)
	root := fs.String("root", ".", "project root")
	output := fs.String("output", defaultPromptOutputDir, "generated prompt client output directory")
	dryRun := fs.Bool("dry-run", false, "preview generated files without writing")
	check := fs.Bool("check", false, "fail if generated files are out of date")
	verbose := fs.Bool("verbose", false, "print scan diagnostics")
	format := fs.String("format", "text", "output format: text or json")
	noColor := fs.Bool("no-color", false, "disable colored output")
	if err := fs.Parse(args); err != nil {
		return Report{}, err
	}
	if *dryRun && *check {
		return Report{}, fmt.Errorf("--dry-run and --check are mutually exclusive")
	}
	report, err := GeneratePrompts(PromptOptions{
		Root:    *root,
		Output:  *output,
		DryRun:  *dryRun,
		Check:   *check,
		Verbose: *verbose,
	})
	if err != nil {
		return report, err
	}
	if err := printReportWithFormat(os.Stdout, report, ReportFormat(*format), !*noColor); err != nil {
		return report, err
	}
	if *check && report.Changed() {
		return report, ErrCheckFailed
	}
	return report, nil
}

func printUsage(w io.Writer) {
	fmt.Fprintln(w, "usage:")
	fmt.Fprintln(w, "  codegen route [-root .] [--dry-run|--check] [--list] [--verbose] [--format text|json] [--no-color]")
	fmt.Fprintln(w, "  codegen repository -model Model [-label 名称] [-scope local_column:table.column:user_column] [-root .] [--dry-run|--check] [--list-models] [--verbose] [--format text|json] [--no-color]")
	fmt.Fprintln(w, "  codegen docs [-root .] [--output docs/openapi.json] [--title title] [--version version] [--dry-run|--check] [--verbose] [--format text|json] [--no-color]")
	fmt.Fprintln(w, "  codegen prompt [-root .] [--output internal/prompts/promptsgen] [--dry-run|--check] [--verbose] [--format text|json] [--no-color]")
	fmt.Fprintln(w, "  codegen doctor [-root .] [--verbose] [--format text|json] [--no-color]")
}

func GenerateRoutes(root string) (Report, error) {
	return GenerateRoutesWithOptions(RouteOptions{Root: root})
}

func GenerateRoutesWithOptions(opts RouteOptions) (Report, error) {
	root := opts.Root
	if root == "" {
		root = "."
	}
	report := Report{Root: root, Command: "route", Mode: generationMode(opts.DryRun, opts.Check)}
	if opts.DryRun && opts.Check {
		return report, fmt.Errorf("--dry-run and --check are mutually exclusive")
	}
	routes, err := scanRoutes(root)
	if err != nil {
		return report, err
	}
	if opts.Verbose {
		report.AddDiagnostic("info", "route.scanned", fmt.Sprintf("scanned %d route directives", len(routes)), "", "")
	}
	if len(routes) == 0 {
		return report, nil
	}
	if err := validateRoutes(routes); err != nil {
		return report, err
	}

	handlers, err := scanHandlers(root)
	if err != nil {
		return report, err
	}
	logics, err := scanLogics(root)
	if err != nil {
		return report, err
	}
	requestDTOs, err := scanRequestDTOs(root)
	if err != nil {
		return report, err
	}

	routesByDomain := map[string][]Route{}
	for _, route := range routes {
		routesByDomain[route.Domain] = append(routesByDomain[route.Domain], route)
	}
	domains := make([]string, 0, len(routesByDomain))
	for domain := range routesByDomain {
		domains = append(domains, domain)
	}
	sort.Strings(domains)

	for _, domain := range domains {
		if err := generateHandler(root, domain, routesByDomain[domain], handlers, requestDTOs, &report); err != nil {
			return report, err
		}
		for _, route := range routesByDomain[domain] {
			key := logicKey(route.Domain, route.HandlerMethod)
			if path, ok := logics[key]; ok {
				report.Add(FileSkipped, path)
				continue
			}
			if logicFileExists(root, route) {
				report.Add(FileSkipped, logicPath(root, route))
				continue
			}
			if err := generateLogic(root, route, &report); err != nil {
				return report, err
			}
			logics[key] = logicPath(root, route)
		}
	}
	return report, nil
}

func generationMode(dryRun bool, check bool) GenerationMode {
	switch {
	case check:
		return ModeCheck
	case dryRun:
		return ModeDryRun
	default:
		return ModeWrite
	}
}

func validateRoutes(routes []Route) error {
	for _, route := range routes {
		if route.Directive.SSE && route.Directive.Event == "" {
			return fmt.Errorf("codegen: %s.%s uses @sse but missing @event <GoType>", route.HandlerType, route.HandlerMethod)
		}
	}
	return nil
}
