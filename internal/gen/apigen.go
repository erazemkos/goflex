package gen

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/printer"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"unicode"
)

const apiImportPath = "github.com/goflex/goflex/pkg/api"

type endpointDecl struct {
	Name        string
	Method      string
	Path        string
	Description string
	Req         string
	Res         string
	ImportPath  string
	Alias       string
}

func Generate(root, only string) (bool, error) {
	if only == "" {
		only = "all"
	}
	if only != "all" && only != "api" {
		return false, nil
	}
	modulePath, err := modulePath(root)
	if err != nil {
		return false, err
	}
	endpoints, err := discoverEndpoints(root, modulePath)
	if err != nil {
		return false, err
	}
	assignAliases(endpoints)
	genDir := filepath.Join(root, "generated")
	if err := os.MkdirAll(genDir, 0o755); err != nil {
		return false, err
	}
	files := map[string][]byte{
		"gen_server.go": renderServer(endpoints),
		"gen_client.go": renderClient(endpoints),
	}
	changed := false
	for name, content := range files {
		formatted, err := format.Source(content)
		if err != nil {
			return changed, fmt.Errorf("format %s: %w\n%s", name, err, content)
		}
		path := filepath.Join(genDir, name)
		old, _ := os.ReadFile(path)
		if bytes.Equal(old, formatted) {
			continue
		}
		if err := os.WriteFile(path, formatted, 0o644); err != nil {
			return changed, err
		}
		changed = true
	}
	return changed, nil
}

func modulePath(root string) (string, error) {
	b, err := os.ReadFile(filepath.Join(root, "go.mod"))
	if err != nil {
		return "", err
	}
	for _, line := range strings.Split(string(b), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 && fields[0] == "module" {
			return fields[1], nil
		}
	}
	return "", fmt.Errorf("module path not found in %s", filepath.Join(root, "go.mod"))
}

func discoverEndpoints(root, modulePath string) ([]endpointDecl, error) {
	var endpoints []endpointDecl
	err := filepath.WalkDir(root, func(dir string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			return nil
		}
		base := filepath.Base(dir)
		if base == ".git" || base == "vendor" || base == "node_modules" || base == "dist" || base == "generated" {
			return filepath.SkipDir
		}
		entries, err := os.ReadDir(dir)
		if err != nil {
			return err
		}
		var goFiles []string
		for _, entry := range entries {
			name := entry.Name()
			if entry.Type().IsRegular() && strings.HasSuffix(name, ".go") && !strings.HasSuffix(name, "_test.go") {
				goFiles = append(goFiles, filepath.Join(dir, name))
			}
		}
		if len(goFiles) == 0 {
			return nil
		}
		rel, err := filepath.Rel(root, dir)
		if err != nil {
			return err
		}
		importPath := modulePath
		if rel != "." {
			importPath += "/" + filepath.ToSlash(rel)
		}
		decls, err := endpointsInPackage(goFiles, importPath)
		if err != nil {
			return err
		}
		endpoints = append(endpoints, decls...)
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(endpoints, func(i, j int) bool {
		if endpoints[i].ImportPath == endpoints[j].ImportPath {
			return endpoints[i].Name < endpoints[j].Name
		}
		return endpoints[i].ImportPath < endpoints[j].ImportPath
	})
	return endpoints, nil
}

func endpointsInPackage(files []string, importPath string) ([]endpointDecl, error) {
	fset := token.NewFileSet()
	var endpoints []endpointDecl
	for _, file := range files {
		parsed, err := parser.ParseFile(fset, file, nil, parser.ParseComments)
		if err != nil {
			return nil, err
		}
		apiAliases := apiImportAliases(parsed)
		if len(apiAliases) == 0 {
			continue
		}
		for _, decl := range parsed.Decls {
			genDecl, ok := decl.(*ast.GenDecl)
			if !ok || genDecl.Tok != token.VAR {
				continue
			}
			for _, spec := range genDecl.Specs {
				valueSpec, ok := spec.(*ast.ValueSpec)
				if !ok {
					continue
				}
				for i, name := range valueSpec.Names {
					if !name.IsExported() {
						continue
					}
					typeExpr := valueSpec.Type
					var lit *ast.CompositeLit
					if i < len(valueSpec.Values) {
						if cl, ok := valueSpec.Values[i].(*ast.CompositeLit); ok {
							lit = cl
							typeExpr = cl.Type
						}
					}
					req, res, ok := endpointTypes(typeExpr, apiAliases, fset, importPath)
					if !ok {
						continue
					}
					method, path, desc := endpointMetadata(lit)
					if method == "" || path == "" {
						continue
					}
					if desc == "" {
						desc = strings.TrimSpace(strings.TrimPrefix(commentText(valueSpec.Doc, genDecl.Doc), name.Name))
					}
					endpoints = append(endpoints, endpointDecl{Name: name.Name, Method: strings.ToUpper(method), Path: path, Description: desc, Req: req, Res: res, ImportPath: importPath})
				}
			}
		}
	}
	return endpoints, nil
}

func apiImportAliases(file *ast.File) map[string]struct{} {
	aliases := map[string]struct{}{}
	for _, imp := range file.Imports {
		pathValue, err := strconv.Unquote(imp.Path.Value)
		if err != nil || pathValue != apiImportPath {
			continue
		}
		if imp.Name != nil {
			aliases[imp.Name.Name] = struct{}{}
		} else {
			aliases["api"] = struct{}{}
		}
	}
	return aliases
}

func endpointTypes(expr ast.Expr, aliases map[string]struct{}, fset *token.FileSet, importPath string) (string, string, bool) {
	idx, ok := expr.(*ast.IndexListExpr)
	if !ok || len(idx.Indices) != 2 {
		return "", "", false
	}
	selector, ok := idx.X.(*ast.SelectorExpr)
	if !ok || selector.Sel.Name != "Endpoint" {
		return "", "", false
	}
	alias, ok := selector.X.(*ast.Ident)
	if !ok {
		return "", "", false
	}
	if _, ok := aliases[alias.Name]; !ok {
		return "", "", false
	}
	return qualifyType(idx.Indices[0], fset, importPath), qualifyType(idx.Indices[1], fset, importPath), true
}

func endpointMetadata(lit *ast.CompositeLit) (method, path, desc string) {
	if lit == nil {
		return "", "", ""
	}
	for _, elt := range lit.Elts {
		kv, ok := elt.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		key, ok := kv.Key.(*ast.Ident)
		if !ok {
			continue
		}
		value, ok := stringLiteral(kv.Value)
		if !ok {
			continue
		}
		switch key.Name {
		case "Method":
			method = value
		case "Path":
			path = value
		case "Description":
			desc = value
		}
	}
	return method, path, desc
}

func stringLiteral(expr ast.Expr) (string, bool) {
	lit, ok := expr.(*ast.BasicLit)
	if !ok || lit.Kind != token.STRING {
		return "", false
	}
	value, err := strconv.Unquote(lit.Value)
	return value, err == nil
}

func qualifyType(expr ast.Expr, fset *token.FileSet, importPath string) string {
	switch e := expr.(type) {
	case *ast.Ident:
		if isBuiltin(e.Name) {
			return e.Name
		}
		return aliasFor(importPath) + "." + e.Name
	case *ast.ArrayType:
		return "[]" + qualifyType(e.Elt, fset, importPath)
	case *ast.StarExpr:
		return "*" + qualifyType(e.X, fset, importPath)
	default:
		return renderExpr(fset, expr)
	}
}

func renderExpr(fset *token.FileSet, expr ast.Expr) string {
	var b bytes.Buffer
	_ = printer.Fprint(&b, fset, expr)
	return b.String()
}

func assignAliases(endpoints []endpointDecl) {
	for i := range endpoints {
		endpoints[i].Alias = aliasFor(endpoints[i].ImportPath)
	}
}

func aliasFor(importPath string) string {
	base := importPath[strings.LastIndex(importPath, "/")+1:]
	var b strings.Builder
	for _, r := range base {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' {
			b.WriteRune(r)
		}
	}
	alias := b.String()
	if alias == "" || !unicode.IsLetter(rune(alias[0])) {
		alias = "pkg" + alias
	}
	return alias
}

func renderServer(endpoints []endpointDecl) []byte {
	var b bytes.Buffer
	b.WriteString("// Code generated by goflex generate; DO NOT EDIT.\n")
	b.WriteString("package generated\n\n")
	b.WriteString("import (\n")
	b.WriteString("\t\"net/http\"\n\n")
	b.WriteString("\t\"github.com/gin-gonic/gin\"\n")
	b.WriteString("\t\"github.com/goflex/goflex/pkg/api\"\n")
	b.WriteString("\t\"github.com/goflex/goflex/pkg/httperr\"\n")
	writeEndpointImports(&b, endpoints)
	b.WriteString(")\n\n")
	b.WriteString("func RegisterRoutes(r *gin.Engine) {\n")
	b.WriteString("\tr.HandleMethodNotAllowed = true\n")
	if len(endpoints) > 0 {
		b.WriteString("\tapiGroup := r.Group(\"/api\")\n")
		for _, ep := range endpoints {
			fmt.Fprintf(&b, "\tapiGroup.Handle(%q, %q, wrap[%s, %s](%s.%s))\n", ep.Method, ep.Path, ep.Req, ep.Res, ep.Alias, ep.Name)
		}
	}
	b.WriteString("}\n\n")
	b.WriteString("func wrap[Req, Res any](ep api.Endpoint[Req, Res]) gin.HandlerFunc {\n")
	b.WriteString("\treturn func(c *gin.Context) {\n")
	b.WriteString("\t\tif c.Request.Method != ep.Method {\n")
	b.WriteString("\t\t\thttperr.Write(c, http.StatusMethodNotAllowed, httperr.New(\"method_not_allowed\", \"method not allowed\", nil))\n")
	b.WriteString("\t\t\treturn\n")
	b.WriteString("\t\t}\n")
	b.WriteString("\t\tvar req Req\n")
	b.WriteString("\t\tif err := api.DecodeRequest(c.Request, c.Param, &req); err != nil {\n")
	b.WriteString("\t\t\thttperr.Write(c, http.StatusBadRequest, httperr.New(\"bad_request\", err.Error(), nil))\n")
	b.WriteString("\t\t\treturn\n")
	b.WriteString("\t\t}\n")
	b.WriteString("\t\tif ep.Handler == nil {\n")
	b.WriteString("\t\t\thttperr.Write(c, http.StatusNotImplemented, httperr.New(\"not_implemented\", \"endpoint handler is not registered\", nil))\n")
	b.WriteString("\t\t\treturn\n")
	b.WriteString("\t\t}\n")
	b.WriteString("\t\tres, err := ep.Handler(c.Request.Context(), req)\n")
	b.WriteString("\t\tif err != nil {\n")
	b.WriteString("\t\t\thttperr.Write(c, http.StatusInternalServerError, err)\n")
	b.WriteString("\t\t\treturn\n")
	b.WriteString("\t\t}\n")
	b.WriteString("\t\tc.JSON(http.StatusOK, res)\n")
	b.WriteString("\t}\n")
	b.WriteString("}\n")
	return b.Bytes()
}

func renderClient(endpoints []endpointDecl) []byte {
	var b bytes.Buffer
	b.WriteString("// Code generated by goflex generate; DO NOT EDIT.\n")
	b.WriteString("package generated\n\n")
	if len(endpoints) == 0 {
		b.WriteString("// Client is a marker for generated typed API clients.\n")
		b.WriteString("type Client struct{}\n")
		return b.Bytes()
	}
	b.WriteString("import (\n")
	b.WriteString("\t\"context\"\n\n")
	b.WriteString("\t\"github.com/goflex/goflex/pkg/apiclient\"\n")
	writeEndpointImports(&b, endpoints)
	b.WriteString(")\n\n")
	for _, ep := range endpoints {
		doc := ep.Description
		if doc == "" {
			doc = fmt.Sprintf("calls %s %s.", ep.Method, ep.Path)
		}
		fmt.Fprintf(&b, "// %s %s\n", ep.Name, sanitizeDoc(doc))
		fmt.Fprintf(&b, "func %s(ctx context.Context, req %s) (%s, error) {\n", ep.Name, ep.Req, ep.Res)
		fmt.Fprintf(&b, "\treturn apiclient.Call[%s, %s](ctx, %s.%s, req)\n", ep.Req, ep.Res, ep.Alias, ep.Name)
		b.WriteString("}\n\n")
	}
	return b.Bytes()
}

func writeEndpointImports(b *bytes.Buffer, endpoints []endpointDecl) {
	seen := map[string]string{}
	for _, ep := range endpoints {
		seen[ep.ImportPath] = ep.Alias
	}
	paths := make([]string, 0, len(seen))
	for p := range seen {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	for _, p := range paths {
		fmt.Fprintf(b, "\t%s %q\n", seen[p], p)
	}
}

func sanitizeDoc(doc string) string {
	doc = strings.Join(strings.Fields(doc), " ")
	if doc == "" {
		return "calls the endpoint."
	}
	if !strings.HasSuffix(doc, ".") {
		doc += "."
	}
	return doc
}

func commentText(groups ...*ast.CommentGroup) string {
	for _, group := range groups {
		if group != nil {
			return group.Text()
		}
	}
	return ""
}

func isBuiltin(name string) bool {
	switch name {
	case "any", "bool", "byte", "comparable", "complex64", "complex128", "error", "float32", "float64", "int", "int8", "int16", "int32", "int64", "rune", "string", "uint", "uint8", "uint16", "uint32", "uint64", "uintptr":
		return true
	default:
		return false
	}
}
