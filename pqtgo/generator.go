package pqtgo

import (
	"bytes"
	"fmt"
	"go/types"
	"io"
	"reflect"
	"sort"
	"strings"

	"github.com/huandu/xstrings"
	"github.com/piotrkowalczuk/pqt"
)

const (
	modeDefault = iota
	modeMandatory
	modeOptional
	modeCriteria

	dirtyAnd = `if dirty {
		wbuf.WriteString(" AND ")
	}
	dirty = true
`
)

// Generator ...
type Generator struct {
	acronyms map[string]string
	imports  []string
	pkg      string
}

// NewGenerator ...
func NewGenerator() *Generator {
	return &Generator{
		pkg: "main",
	}
}

// SetAcronyms ...
func (g *Generator) SetAcronyms(acronyms map[string]string) *Generator {
	g.acronyms = acronyms

	return g
}

// SetImports ...
func (g *Generator) SetImports(imports ...string) *Generator {
	g.imports = imports

	return g
}

// AddImport ...
func (g *Generator) AddImport(i string) *Generator {
	if g.imports == nil {
		g.imports = make([]string, 0, 1)
	}

	g.imports = append(g.imports, i)
	return g
}

// SetPackage ...
func (g *Generator) SetPackage(pkg string) *Generator {
	g.pkg = pkg

	return g
}

// Generate ...
func (g *Generator) Generate(s *pqt.Schema) ([]byte, error) {
	code, err := g.generate(s)
	if err != nil {
		return nil, err
	}

	return code.Bytes(), nil
}

// GenerateTo ...
func (g *Generator) GenerateTo(s *pqt.Schema, w io.Writer) error {
	code, err := g.generate(s)
	if err != nil {
		return err
	}

	_, err = code.WriteTo(w)
	return err
}

func (g *Generator) generate(s *pqt.Schema) (*bytes.Buffer, error) {
	b := bytes.NewBuffer(nil)

	g.generatePackage(b)
	g.generateImports(b, s)
	for _, t := range s.Tables {
		g.generateConstants(b, t)
		g.generateColumns(b, t)
		g.generateEntity(b, t)
		g.generateEntityProp(b, t)
		g.generateEntityProps(b, t)
		g.generateIterator(b, t)
		g.generateCriteria(b, t)
		g.generateCriteriaWriteSQL(b, t)
		g.generatePatch(b, t)
		g.generateRepository(b, t)
	}

	return b, nil
}

func (g *Generator) generatePackage(code *bytes.Buffer) {
	fmt.Fprintf(code, "package %s\n", g.pkg)
}

func (g *Generator) generateImports(code *bytes.Buffer, schema *pqt.Schema) {
	imports := []string{
		"github.com/go-kit/kit/log",
		"github.com/m4rw3r/uuid",
	}
	imports = append(imports, g.imports...)
	for _, t := range schema.Tables {
		for _, c := range t.Columns {
			if ct, ok := c.Type.(CustomType); ok {
				imports = append(imports, ct.mandatoryTypeOf.PkgPath())
				imports = append(imports, ct.mandatoryTypeOf.PkgPath())
				imports = append(imports, ct.mandatoryTypeOf.PkgPath())
			}
		}
	}

	code.WriteString("import (\n")
	for _, imp := range imports {
		code.WriteRune('"')
		fmt.Fprint(code, imp)
		code.WriteRune('"')
		code.WriteRune('\n')
	}
	code.WriteString(")\n")
}

func (g *Generator) generateEntity(w io.Writer, t *pqt.Table) {
	fmt.Fprintf(w, "type %sEntity struct{\n", g.private(t.Name))

	for _, c := range t.Columns {
		fmt.Fprintf(w, "%s %s\n", g.public(c.Name), g.generateColumnTypeString(c, modeDefault))
	}

	for _, r := range t.OwnedRelationships {
		switch r.Type {
		case pqt.RelationshipTypeOneToMany:
			if r.InversedName != "" {
				fmt.Fprintf(w, "%s", g.public(r.InversedName))
			} else {
				fmt.Fprintf(w, "%ss", g.public(r.InversedTable.Name))
			}
			fmt.Fprintf(w, " []*%sEntity\n", g.private(r.InversedTable.Name))
		case pqt.RelationshipTypeOneToOne, pqt.RelationshipTypeManyToOne:
			if r.InversedName != "" {
				fmt.Fprintf(w, "%s", g.public(r.InversedName))
			} else {
				fmt.Fprintf(w, "%s", g.public(r.InversedTable.Name))
			}
			fmt.Fprintf(w, " *%sEntity\n", g.private(r.InversedTable.Name))
		case pqt.RelationshipTypeManyToMany:
			if r.OwnerName != "" {
				fmt.Fprintf(w, "%s", g.public(r.OwnerName))
			} else {
				fmt.Fprintf(w, "%s", g.public(r.OwnerTable.Name))
			}
			fmt.Fprintf(w, " *%sEntity\n", g.private(r.OwnerTable.Name))

			if r.InversedName != "" {
				fmt.Fprintf(w, "%s", g.public(r.InversedName))
			} else {
				fmt.Fprintf(w, "%s", g.public(r.InversedTable.Name))
			}
			fmt.Fprintf(w, " *%sEntity\n", g.private(r.InversedTable.Name))
		}
	}

	for _, r := range t.InversedRelationships {
		switch r.Type {
		case pqt.RelationshipTypeOneToMany:
			if r.OwnerName != "" {
				fmt.Fprintf(w, "%s", g.public(r.OwnerName))
			} else {
				fmt.Fprintf(w, "%s", g.public(r.OwnerTable.Name))
			}
			fmt.Fprintf(w, " *%sEntity\n", g.private(r.OwnerTable.Name))
		case pqt.RelationshipTypeOneToOne:
			if r.OwnerName != "" {
				fmt.Fprintf(w, "%s", g.public(r.OwnerName))
			} else {
				fmt.Fprintf(w, "%ss", g.public(r.OwnerTable.Name))
			}
			fmt.Fprintf(w, " *%sEntity\n", g.private(r.OwnerTable.Name))
		case pqt.RelationshipTypeManyToOne:
			if r.OwnerName != "" {
				fmt.Fprintf(w, "%s", g.public(r.OwnerName))
			} else {
				fmt.Fprintf(w, "%ss", g.public(r.OwnerTable.Name))
			}
			fmt.Fprintf(w, " []*%sEntity\n", g.private(r.OwnerTable.Name))
		}
	}

	for _, r := range t.ManyToManyRelationships {
		if r.Type != pqt.RelationshipTypeManyToMany {
			continue
		}

		switch {
		case r.OwnerTable == t:
			if r.InversedName != "" {
				fmt.Fprintf(w, "%s", g.public(r.InversedName))
			} else {
				fmt.Fprintf(w, "%s", g.public(r.InversedTable.Name))
			}
			fmt.Fprintf(w, " []*%sEntity\n", g.private(r.InversedTable.Name))
		case r.InversedTable == t:
			if r.OwnerName != "" {
				fmt.Fprintf(w, "%s", g.public(r.OwnerName))
			} else {
				fmt.Fprintf(w, "%ss", g.public(r.OwnerTable.Name))
			}
			fmt.Fprintf(w, " []*%sEntity\n", g.private(r.OwnerTable.Name))
		}
	}

	fmt.Fprintln(w, "}\n")
}

func (g *Generator) generateEntityProp(w io.Writer, t *pqt.Table) {
	fmt.Fprintf(w, "func (e *%sEntity) Prop(cn string) (interface{}, bool) {\n", g.private(t.Name))
	fmt.Fprintln(w, "switch cn {")
	for _, c := range t.Columns {
		fmt.Fprintf(w, "case %s:\n", g.columnNameWithTableName(t.Name, c.Name))
		if g.canBeNil(c, modeDefault) {
			fmt.Fprintf(w, "return e.%s, true\n", g.public(c.Name))
		} else {
			fmt.Fprintf(w, "return &e.%s, true\n", g.public(c.Name))
		}
	}
	fmt.Fprint(w, "default:\n")
	fmt.Fprint(w, "return nil, false\n")
	fmt.Fprint(w, "}\n}\n")
}

func (g *Generator) generateEntityProps(w io.Writer, t *pqt.Table) {
	fmt.Fprintf(w, "func (e *%sEntity) Props(cns ...string) ([]interface{}, error) {\n", g.private(t.Name))
	fmt.Fprintf(w, `
		res := make([]interface{}, 0, len(cns))
		for _, cn := range cns {
			if prop, ok := e.Prop(cn); ok {
				res = append(res, prop)
			} else {
				return nil, fmt.Errorf("unexpected column provided: %%s", cn)
			}
		}
		return res, nil`)
	fmt.Fprint(w, "\n}\n")
}
func (g *Generator) generateIterator(w io.Writer, t *pqt.Table) {
	entityName := g.private(t.Name)
	fmt.Fprintf(w, `

// %sIterator is not thread safe.
type %sIterator struct {
	rows *sql.Rows
	cols []string
}

func (i *%sIterator) Next() bool {
	return i.rows.Next()
}

func (i *%sIterator) Close() error {
	return i.rows.Close()
}

func (i *%sIterator) Err() error {
	return i.rows.Err()
}

// Columns is wrapper around sql.Rows.Columns method, that also cache outpu inside iterator.
func (i *%sIterator) Columns() ([]string, error) {
	if i.cols == nil {
		cols, err := i.rows.Columns()
		if err != nil {
			return nil, err
		}
		i.cols = cols
	}
	return i.cols, nil
}

// Ent is wrapper arround %s method that makes iterator more generic.
func (i *%sIterator) Ent() (interface{}, error) {
	return i.%s()
}

func (i *%sIterator) %s() (*%sEntity, error) {
	var ent %sEntity
	cols, err := i.rows.Columns()
	if err != nil {
		return nil, err
	}

	props, err := ent.Props(cols...)
	if err != nil {
		return nil, err
	}
	if err := i.rows.Scan(props...); err != nil {
		return nil, err
	}
	return &ent, nil
}
`, entityName, entityName, entityName, entityName, entityName, entityName, entityName, entityName, g.public(t.Name), entityName, g.public(t.Name), entityName, entityName)
}

func (g *Generator) generateCriteria(w io.Writer, t *pqt.Table) {
	fmt.Fprintf(w, "type %sCriteria struct {\n", g.private(t.Name))
	fmt.Fprintln(w, "offset, limit int64")
	fmt.Fprintln(w, "sort map[string]bool")

ColumnLoop:
	for _, c := range t.Columns {
		if g.shouldBeColumnIgnoredForCriteria(c) {
			continue ColumnLoop
		}

		fmt.Fprintf(w, "%s %s\n", g.private(c.Name), g.generateColumnTypeString(c, modeCriteria))
	}
	fmt.Fprintln(w, "}\n")
}

func (g *Generator) generatePatch(w io.Writer, t *pqt.Table) {
	pk, ok := t.PrimaryKey()
	if !ok {
		return
	}
	fmt.Fprintf(w, "type %sPatch struct {\n", g.private(t.Name))
	fmt.Fprintf(w, "%s %s\n", g.private(pk.Name), g.generateColumnTypeString(pk, modeMandatory))

ArgumentsLoop:
	for _, c := range t.Columns {
		if c.PrimaryKey {
			continue ArgumentsLoop
		}

		fmt.Fprintf(w, "%s %s\n", g.private(c.Name), g.generateColumnTypeString(c, modeOptional))
	}
	fmt.Fprintln(w, "}\n")
}

func (g *Generator) generateColumnType(w io.Writer, c *pqt.Column, m int32) {
	fmt.Fprintln(w, g.generateColumnTypeString(c, m))
}

func (g *Generator) generateColumnTypeString(c *pqt.Column, m int32) string {
	switch m {
	case modeCriteria:
	case modeMandatory:
	case modeOptional:
	default:
		if c.NotNull || c.PrimaryKey {
			m = modeMandatory
		}
	}

	return g.generateType(c.Type, m)
}

func (g *Generator) generateType(t pqt.Type, m int32) string {
	switch tt := t.(type) {
	case pqt.MappableType:
		for _, mt := range tt.Mapping {
			return g.generateType(mt, m)
		}
		return ""
	case BuiltinType:
		return generateBuiltinType(tt, m)
	case pqt.BaseType:
		return generateBaseType(tt, m)
	case CustomType:
		return generateCustomType(tt, m)
	default:
		return ""
	}
}

func (g *Generator) generateConstants(code *bytes.Buffer, table *pqt.Table) {
	code.WriteString("const (\n")
	g.generateConstantsColumns(code, table)
	g.generateConstantsConstraints(code, table)
	code.WriteString(")\n")
}

func (g *Generator) generateConstantsColumns(code *bytes.Buffer, table *pqt.Table) {
	fmt.Fprintf(code, `table%s = "%s"`, g.public(table.Name), table.FullName())
	code.WriteRune('\n')

	for _, name := range sortedColumns(table.Columns) {
		fmt.Fprintf(code, `table%sColumn%s = "%s"`, g.public(table.Name), g.public(name), name)
		code.WriteRune('\n')
	}
}

func (g *Generator) generateConstantsConstraints(code *bytes.Buffer, table *pqt.Table) {
	for _, c := range tableConstraints(table) {
		name := fmt.Sprintf("%s", pqt.JoinColumns(c.Columns, "_"))
		switch c.Type {
		case pqt.ConstraintTypeCheck:
			fmt.Fprintf(code, `table%sConstraint%sCheck = "%s"`, g.public(table.Name), g.public(name), c.String())
		case pqt.ConstraintTypePrimaryKey:
			fmt.Fprintf(code, `table%sConstraintPrimaryKey = "%s"`, g.public(table.Name), c.String())
		case pqt.ConstraintTypeForeignKey:
			fmt.Fprintf(code, `table%sConstraint%sForeignKey = "%s"`, g.public(table.Name), g.public(name), c.String())
		case pqt.ConstraintTypeExclusion:
			fmt.Fprintf(code, `table%sConstraint%sExclusion = "%s"`, g.public(table.Name), g.public(name), c.String())
		case pqt.ConstraintTypeUnique:
			fmt.Fprintf(code, `table%sConstraint%sUnique = "%s"`, g.public(table.Name), g.public(name), c.String())
		case pqt.ConstraintTypeIndex:
			fmt.Fprintf(code, `table%sConstraint%sIndex = "%s"`, g.public(table.Name), g.public(name), c.String())
		}

		code.WriteRune('\n')
	}
}

func (g *Generator) generateColumns(code *bytes.Buffer, table *pqt.Table) {
	code.WriteString("var (\n")
	code.WriteString("table")
	code.WriteString(g.public(table.Name))
	code.WriteString("Columns = []string{\n")

	for _, name := range sortedColumns(table.Columns) {
		g.writeTableNameColumnNameTo(code, table.Name, name)
		code.WriteRune(',')
		code.WriteRune('\n')
	}
	code.WriteString("}")
	code.WriteString(")\n")
}

func (g *Generator) generateRepository(b *bytes.Buffer, t *pqt.Table) {
	fmt.Fprintf(b, `
		type %sRepositoryBase struct {
			table string
			columns []string
			db *sql.DB
			dbg bool
			log log.Logger
		}
	`, g.private(t.Name))
	g.generateRepositoryScanRows(b, t)
	g.generateRepositoryCount(b, t)
	g.generateRepositoryFind(b, t)
	g.generateRepositoryFindIter(b, t)
	g.generateRepositoryFindOneByPrimaryKey(b, t)
	g.generateRepositoryInsert(b, t)
	g.generateRepositoryUpdateByPrimaryKey(b, t)
	g.generateRepositoryDeleteByPrimaryKey(b, t)
}

func (g *Generator) generateRepositoryFindPropertyQuery(w io.Writer, c *pqt.Column) {
	columnNamePrivate := g.private(c.Name)
	columnNameWithTable := g.columnNameWithTableName(c.Table.Name, c.Name)

	if !g.generateRepositoryFindPropertyQueryByGoType(w, c, g.generateColumnTypeString(c, modeCriteria), columnNamePrivate, columnNameWithTable) {
		fmt.Fprintf(w, " if c.%s != nil {", g.private(c.Name))
		fmt.Fprintf(w, dirtyAnd)
		fmt.Fprintf(w, `if wrt, err = wbuf.WriteString(%s); err != nil {
			return
		}
		wr += int64(wrt)
		`, g.columnNameWithTableName(c.Table.Name, c.Name))
		fmt.Fprintf(w, `if wrt, err = wbuf.WriteString("="); err != nil {
			return
		}
		wr += int64(wrt)
		`)
		fmt.Fprintln(w, `if wrt64, err = pw.WriteTo(wbuf); err != nil {
			return
		}
		wr += wrt64
		`)
		fmt.Fprintf(w, `args.Add(c.%s)
		}`, g.private(c.Name))
	}
}

func (g *Generator) generateRepositoryFindPropertyQueryByGoType(w io.Writer, col *pqt.Column, goType, columnNamePrivate, columnNameWithTable string) (done bool) {
	switch goType {
	case "uuid.UUID":
		fmt.Fprintf(w, `
			if !c.%s.IsZero() {
				%s
				if wrt, err = wbuf.WriteString(%s); err != nil {
					return
				}
				wr += int64(wrt)
				if wrt, err = wbuf.WriteString("!="); err != nil {
					return
				}
				wr += int64(wrt)
				if wrt64, err = pw.WriteTo(wbuf); err != nil {
					return
				}
				wr += wrt64
				args.Add(c.%s)
			}
		`, columnNamePrivate, dirtyAnd, columnNameWithTable, columnNamePrivate)
	case "*qtypes.Timestamp":
		fmt.Fprintf(w, `
				if c.%s != nil && c.%s.Valid {
					%st1 := c.%s.Value()
					if %st1 != nil {
						%s1, err := ptypes.Timestamp(%st1)
						if err != nil {
							return wr, err
						}
						switch c.%s.Type {
						case qtypes.NumericQueryType_NOT_A_NUMBER:
							%s
							wbuf.WriteString(%s)
							if c.%s.Negation {
								wbuf.WriteString(" IS NOT NULL ")
							} else {
								wbuf.WriteString(" IS NULL ")
							}
						case qtypes.NumericQueryType_EQUAL:
							%s
							wbuf.WriteString(%s)
							if c.%s.Negation {
								wbuf.WriteString("<>")
							} else {
								wbuf.WriteString("=")
							}
							pw.WriteTo(wbuf)
							args.Add(c.%s.Value())
						case qtypes.NumericQueryType_GREATER:
							%s
							wbuf.WriteString(%s)
							wbuf.WriteString(">")
							pw.WriteTo(wbuf)
							args.Add(c.%s.Value())
						case qtypes.NumericQueryType_GREATER_EQUAL:
							%s
							wbuf.WriteString(%s)
							wbuf.WriteString(">=")
							pw.WriteTo(wbuf)
							args.Add(c.%s.Value())
						case qtypes.NumericQueryType_LESS:
							%s
							wbuf.WriteString(%s)
							wbuf.WriteString("<")
							pw.WriteTo(wbuf)
							args.Add(c.%s.Value())
						case qtypes.NumericQueryType_LESS_EQUAL:
							%s
							wbuf.WriteString(%s)
							wbuf.WriteString("<=")
							pw.WriteTo(wbuf)
							args.Add(c.%s.Value())
						case qtypes.NumericQueryType_IN:
							if len(c.%s.Values) >0 {
								%s
								wbuf.WriteString(%s)
								wbuf.WriteString(" IN (")
								for i, v := range c.%s.Values {
									if i != 0 {
										wbuf.WriteString(",")
									}
									pw.WriteTo(wbuf)
									args.Add(v)
								}
								wbuf.WriteString(") ")
							}
						case qtypes.NumericQueryType_BETWEEN:
							%s
							%st2 := c.%s.Values[1]
							if %st2 != nil {
								%s2, err := ptypes.Timestamp(%st2)
								if err != nil {
									return wr, err
								}
								wbuf.WriteString(%s)
								wbuf.WriteString(" > ")
								pw.WriteTo(wbuf)
								args.Add(%s1)
								wbuf.WriteString(" AND ")
								wbuf.WriteString(%s)
								wbuf.WriteString(" < ")
								pw.WriteTo(wbuf)
								args.Add(%s2)
							}
						}
					}
				}
`,
			columnNamePrivate, columnNamePrivate,
			columnNamePrivate, columnNamePrivate,
			columnNamePrivate,
			columnNamePrivate,
			columnNamePrivate, columnNamePrivate,
			// NOT A NUMBER
			dirtyAnd,
			columnNameWithTable,
			columnNamePrivate,
			// EQUAL
			dirtyAnd,
			columnNameWithTable, columnNamePrivate, columnNamePrivate,
			// GREATER
			dirtyAnd,
			columnNameWithTable, columnNamePrivate,
			// GREATER EQUAL
			dirtyAnd,
			columnNameWithTable, columnNamePrivate,
			// LESS
			dirtyAnd,
			columnNameWithTable, columnNamePrivate,
			// LESS EQUAL
			dirtyAnd,
			columnNameWithTable, columnNamePrivate,
			// IN
			columnNamePrivate,
			dirtyAnd,
			columnNameWithTable, columnNamePrivate,
			// BETWEEN
			dirtyAnd,
			columnNamePrivate, columnNamePrivate,
			columnNamePrivate, columnNamePrivate,
			columnNamePrivate,
			columnNameWithTable, columnNamePrivate,
			columnNameWithTable, columnNamePrivate,
		)
	case "*qtypes.Int64", "*qtypes.Float64":
		fmt.Fprintf(w, `
				if c.%s != nil && c.%s.Valid {
					switch c.%s.Type {
					case qtypes.NumericQueryType_NOT_A_NUMBER:
						%s
						wbuf.WriteString(%s)
						if c.%s.Negation {
							wbuf.WriteString(" IS NOT NULL ")
						} else {
							wbuf.WriteString(" IS NULL ")
						}
					case qtypes.NumericQueryType_EQUAL:
						%s
						wbuf.WriteString(%s)
						if c.%s.Negation {
							wbuf.WriteString(" <> ")
						} else {
							wbuf.WriteString("=")
						}
						pw.WriteTo(wbuf)
						args.Add(c.%s.Value())
					case qtypes.NumericQueryType_GREATER:
						%s
						wbuf.WriteString(%s)
						if c.%s.Negation {
							wbuf.WriteString(" <= ")
						} else {
							wbuf.WriteString(" > ")
						}
						pw.WriteTo(wbuf)
						args.Add(c.%s.Value())
					case qtypes.NumericQueryType_GREATER_EQUAL:
						%s
						wbuf.WriteString(%s)
						if c.%s.Negation {
							wbuf.WriteString(" < ")
						} else {
							wbuf.WriteString(" >= ")
						}
						pw.WriteTo(wbuf)
						args.Add(c.%s.Value())
					case qtypes.NumericQueryType_LESS:
						%s
						wbuf.WriteString(%s)
						if c.%s.Negation {
							wbuf.WriteString(" >= ")
						} else {
							wbuf.WriteString(" < ")
						}
						pw.WriteTo(wbuf)
						args.Add(c.%s)
					case qtypes.NumericQueryType_LESS_EQUAL:
						%s
						wbuf.WriteString(%s)
						if c.%s.Negation {
							wbuf.WriteString(" > ")
						} else {
							wbuf.WriteString(" <= ")
						}
						pw.WriteTo(wbuf)
						args.Add(c.%s.Value())
					case qtypes.NumericQueryType_IN:
						if len(c.%s.Values) >0 {
							%s
							wbuf.WriteString(%s)
							if c.%s.Negation {
								wbuf.WriteString(" NOT IN (")
							} else {
								wbuf.WriteString(" IN (")
							}
							for i, v := range c.%s.Values {
								if i != 0 {
									wbuf.WriteString(",")
								}
								pw.WriteTo(wbuf)
								args.Add(v)
							}
							wbuf.WriteString(") ")
						}
					case qtypes.NumericQueryType_BETWEEN:
						%s
						wbuf.WriteString(%s)
						if c.%s.Negation {
							wbuf.WriteString(" <= ")
						} else {
							wbuf.WriteString(" > ")
						}
						pw.WriteTo(wbuf)
						args.Add(c.%s.Values[0])
						wbuf.WriteString(" AND ")
						wbuf.WriteString(%s)
						if c.%s.Negation {
							wbuf.WriteString(" >= ")
						} else {
							wbuf.WriteString(" < ")
						}
						pw.WriteTo(wbuf)
						args.Add(c.%s.Values[1])
					}
				}
`,
			columnNamePrivate, columnNamePrivate,
			columnNamePrivate,
			// NOT A NUMBER
			dirtyAnd,
			columnNameWithTable,
			columnNamePrivate,
			// EQUAL
			dirtyAnd,
			columnNameWithTable, columnNamePrivate, columnNamePrivate,
			// GREATER
			dirtyAnd,
			columnNameWithTable, columnNamePrivate, columnNamePrivate,
			// GREATER EQUAL
			dirtyAnd,
			columnNameWithTable, columnNamePrivate, columnNamePrivate,
			// LESS
			dirtyAnd,
			columnNameWithTable, columnNamePrivate, columnNamePrivate,
			// LESS EQUAL
			dirtyAnd,
			columnNameWithTable, columnNamePrivate, columnNamePrivate,
			// IN
			columnNamePrivate,
			dirtyAnd,
			columnNameWithTable,
			columnNamePrivate, columnNamePrivate,
			// BETWEEN
			dirtyAnd,
			columnNameWithTable,
			columnNamePrivate, columnNamePrivate,
			columnNameWithTable,
			columnNamePrivate, columnNamePrivate,
		)
	case "*qtypes.String":
		fmt.Fprintf(w, `
				if c.%s != nil && c.%s.Valid {
					switch c.%s.Type {
					case qtypes.TextQueryType_NOT_A_TEXT:
						%s
						wbuf.WriteString(%s)
						if c.%s.Negation {
							wbuf.WriteString(" IS NOT NULL ")
						} else {
							wbuf.WriteString(" IS NULL ")
						}
					case qtypes.TextQueryType_EXACT:
						%s
						wbuf.WriteString(%s)
						if c.%s.Negation {
							wbuf.WriteString(" <> ")
						} else {
							wbuf.WriteString(" = ")
						}
						pw.WriteTo(wbuf)
						args.Add(c.%s.Value())
					case qtypes.TextQueryType_SUBSTRING:
						%s
						wbuf.WriteString(%s)
						if c.%s.Negation {
							wbuf.WriteString(" NOT LIKE ")
						} else {
							wbuf.WriteString(" LIKE ")
						}
						pw.WriteTo(wbuf)
						args.Add(fmt.Sprintf("%%%%%%s%%%%", c.%s.Value()))
					case qtypes.TextQueryType_HAS_PREFIX:
						%s
						wbuf.WriteString(%s)
						if c.%s.Negation {
							wbuf.WriteString(" NOT LIKE ")
						} else {
							wbuf.WriteString(" LIKE ")
						}
						pw.WriteTo(wbuf)
						args.Add(fmt.Sprintf("%%s%%%%", c.%s.Value()))
					case qtypes.TextQueryType_HAS_SUFFIX:
						%s
						wbuf.WriteString(%s)
						if c.%s.Negation {
							wbuf.WriteString(" NOT LIKE ")
						} else {
							wbuf.WriteString(" LIKE ")
						}
						pw.WriteTo(wbuf)
						args.Add(fmt.Sprintf("%%%%%%s", c.%s.Value()))
					}
				}
`,
			columnNamePrivate, columnNamePrivate,
			columnNamePrivate,
			// NOT A TEXT
			dirtyAnd,
			columnNameWithTable, columnNamePrivate,
			// EXACT
			dirtyAnd,
			columnNameWithTable, columnNamePrivate, columnNamePrivate,
			// SUBSTRING
			dirtyAnd,
			columnNameWithTable, columnNamePrivate, columnNamePrivate,
			// HAS PREFIX
			dirtyAnd,
			columnNameWithTable, columnNamePrivate, columnNamePrivate,
			// HAS SUFFIX
			dirtyAnd,
			columnNameWithTable, columnNamePrivate, columnNamePrivate,
		)
	default:
		if strings.HasPrefix(goType, "*ntypes.") {
			fmt.Fprintf(w, " if c.%s != nil && c.%s.Valid {", columnNamePrivate, columnNamePrivate)
			fmt.Fprintf(w, dirtyAnd)
			fmt.Fprintf(w, "wbuf.WriteString(%s)\n", columnNameWithTable)
			fmt.Fprintf(w, `wbuf.WriteString("=")
`)
			fmt.Fprintln(w, `pw.WriteTo(wbuf)`)
			fmt.Fprintf(w, `args.Add(c.%s)
		}`, columnNamePrivate)

			return true
		}
		return
	}
	return true
}

func (g *Generator) generateRepositoryFindSingleExpression(w io.Writer, c *pqt.Column) {
	if mappt, ok := c.Type.(pqt.MappableType); ok {
	MappingLoop:
		for _, mt := range mappt.Mapping {
			switch mtt := mt.(type) {
			case CustomType:
				if gct := generateCustomType(mtt, modeCriteria); strings.HasPrefix(gct, "*qtypes.") {
					break MappingLoop
				}

				if mtt.criteriaTypeOf == nil {
					fmt.Printf("%s.%s: criteria type of nil\n", c.Table.FullName(), c.Name)
					return
				}
				if mtt.criteriaTypeOf.Kind() == reflect.Invalid {
					fmt.Printf("%s.%s: criteria invalid type\n", c.Table.FullName(), c.Name)
					return
				}

				columnNamePrivate := g.private(c.Name)
				columnNameWithTable := g.columnNameWithTableName(c.Table.Name, c.Name)
				zero := reflect.Zero(mtt.criteriaTypeOf)

				// Checks if custom type implements Criterion interface.
				// If it's true then just use it.
				if zero.CanInterface() {
					if _, ok := zero.Interface().(Criterion); ok {
						fmt.Fprintf(w, `
							if wrt64, err = c.%s.Criteria(wbuf, pw, args, %s); err != nil {
								return
							}
							wr += wrt64
							if args.Len() != 0 {
								dirty = true
							}
						`, columnNamePrivate, columnNameWithTable)
						return
					}
				}

				switch zero.Kind() {
				case reflect.Map:
					// TODO: implement
					return
				case reflect.Struct:
					for i := 0; i < zero.NumField(); i++ {
						field := zero.Field(i)
						fieldName := columnNamePrivate + "." + zero.Type().Field(i).Name
						fieldJSONName := strings.Split(zero.Type().Field(i).Tag.Get("json"), ",")[0]
						columnNameWithTableAndJSONSelector := fmt.Sprintf(`%s + " -> '%s'"`, columnNameWithTable, fieldJSONName)

						// If struct is nil, it's properties should not be accessed.
						fmt.Fprintf(w, `if c.%s != nil {
						`, g.private(c.Name))
						g.generateRepositoryFindPropertyQueryByGoType(w, c, field.Type().String(), fieldName, columnNameWithTableAndJSONSelector)
						fmt.Fprintf(w, `}
						`)
					}
				}
			default:
				g.generateRepositoryFindPropertyQuery(w, c)
			}
		}
	} else {
		g.generateRepositoryFindPropertyQuery(w, c)
	}
	fmt.Fprintln(w, "")
}

func (g *Generator) generateCriteriaWriteSQL(w io.Writer, t *pqt.Table) {
	entityName := g.private(t.Name)
	fmt.Fprintf(w, `func (c *%sCriteria) WriteSQL(b *bytes.Buffer, pw *pqtgo.PlaceholderWriter, args *pqtgo.Arguments) (wr int64, err error) {
		var (
			wrt int
			wrt64 int64
			dirty bool
		)

		wbuf := bytes.NewBuffer(nil)
`, entityName)
	for _, c := range t.Columns {
		if g.shouldBeColumnIgnoredForCriteria(c) {
			continue
		}

		g.generateRepositoryFindSingleExpression(w, c)
	}
	fmt.Fprintf(w, `
	if dirty {
		if wrt, err = b.WriteString(" WHERE "); err != nil {
			return
		}
		wr += int64(wrt)
		if wrt64, err = wbuf.WriteTo(b); err != nil {
			return
		}
		wr += wrt64
	}

	if c.offset > 0 {
		b.WriteString(" OFFSET ")
		if wrt64, err = pw.WriteTo(b); err != nil {
			return
		}
		wr += wrt64
		args.Add(c.offset)
	}
	if c.limit > 0 {
		b.WriteString(" LIMIT ")
		if wrt64, err = pw.WriteTo(b); err != nil {
			return
		}
		wr += wrt64
		args.Add(c.limit)
	}

	return
}
`)
}

func (g *Generator) generateRepositoryScanRows(w io.Writer, t *pqt.Table) {
	entityName := g.private(t.Name)
	fmt.Fprintf(w, `func Scan%sRows(rows *sql.Rows) ([]*%sEntity, error) {
	`, g.public(t.Name), entityName)
	fmt.Fprintf(w, `var (
		entities []*%sEntity
		err error
	)
	for rows.Next() {
		var ent %sEntity
		err = rows.Scan(
	`, entityName, entityName)
	for _, c := range t.Columns {
		fmt.Fprintf(w, "&ent.%s,\n", g.public(c.Name))
	}
	fmt.Fprint(w, `)
			if err != nil {
				return nil, err
			}

			entities = append(entities, &ent)
		}
		if rows.Err() != nil {
			return nil, rows.Err()
		}

		return entities, nil
	}

	`)
}
func (g *Generator) generateRepositoryFindBody(w io.Writer, t *pqt.Table) {
	fmt.Fprintf(w, `
	qbuf := bytes.NewBuffer(nil)
	qbuf.WriteString("SELECT ")
	qbuf.WriteString(strings.Join(r.columns, ", "))
	qbuf.WriteString(" FROM ")
	qbuf.WriteString(r.table)

	pw := pqtgo.NewPlaceholderWriter()
	args := pqtgo.NewArguments(0)

	if _, err := c.WriteSQL(qbuf, pw, args); err != nil {
		return nil, err
	}

	if r.dbg {
		if err := r.log.Log("msg", qbuf.String(), "function", "Find"); err != nil {
			return nil, err
		}
	}

	rows, err := r.db.Query(qbuf.String(), args.Slice()...)
	if err != nil {
		return nil, err
	}
`)
}

func (g *Generator) generateRepositoryFind(w io.Writer, t *pqt.Table) {
	entityName := g.private(t.Name)

	fmt.Fprintf(w, `func (r *%sRepositoryBase) Find(c *%sCriteria) ([]*%sEntity, error) {
`, entityName, entityName, entityName)
	g.generateRepositoryFindBody(w, t)
	fmt.Fprintf(w, `
	defer rows.Close()

	return Scan%sRows(rows)
}
`, g.public(t.Name))
}

func (g *Generator) generateRepositoryFindIter(w io.Writer, t *pqt.Table) {
	entityName := g.private(t.Name)

	fmt.Fprintf(w, `func (r *%sRepositoryBase) FindIter(c *%sCriteria) (*%sIterator, error) {
`, entityName, entityName, entityName)
	g.generateRepositoryFindBody(w, t)
	fmt.Fprintf(w, `

	return &%sIterator{rows: rows}, nil
}
`, g.private(t.Name))
}

func (g *Generator) generateRepositoryCount(w io.Writer, t *pqt.Table) {
	entityName := g.private(t.Name)

	fmt.Fprintf(w, `func (r *%sRepositoryBase) Count(c *%sCriteria) (int64, error) {
`, entityName, entityName)
	fmt.Fprintf(w, `
	qbuf := bytes.NewBuffer(nil)
	qbuf.WriteString("SELECT COUNT(*) FROM ")
	qbuf.WriteString(r.table)
	pw := pqtgo.NewPlaceholderWriter()
	args := pqtgo.NewArguments(0)

	if _, err := c.WriteSQL(qbuf, pw, args); err != nil {
		return 0, err
	}
	if r.dbg {
		if err := r.log.Log("msg", qbuf.String(), "function", "Count"); err != nil {
			return 0, err
		}
	}

	var count int64
	err := r.db.QueryRow(qbuf.String(), args.Slice()...).Scan(&count)
	if err != nil {
		return 0, err
	}
	return count, nil
}
`)
}

func (g *Generator) generateRepositoryFindOneByPrimaryKey(code *bytes.Buffer,
	table *pqt.Table) {
	entityName := g.private(table.Name)
	pk, ok := table.PrimaryKey()
	if !ok {
		return
	}

	fmt.Fprintf(code, `func (r *%sRepositoryBase) FindOneBy%s(%s %s) (*%sEntity, error) {`, entityName, g.public(pk.Name), g.private(pk.Name), g.generateColumnTypeString(pk, modeMandatory), entityName)
	fmt.Fprintf(code, `var (
		query string
		entity %sEntity
	)`, entityName)
	code.WriteRune('\n')
	fmt.Fprintf(code, "query = `SELECT ")
	for i, c := range table.Columns {
		fmt.Fprintf(code, "%s", c.Name)
		if i != len(table.Columns)-1 {
			code.WriteRune(',')
		}
		code.WriteRune('\n')
	}
	fmt.Fprintf(code, " FROM %s WHERE %s = $1`", table.FullName(), pk.Name)

	fmt.Fprintf(code, `
	err := r.db.QueryRow(query, %s).Scan(
	`, g.private(pk.Name))
	for _, c := range table.Columns {
		fmt.Fprintf(code, "&entity.%s,\n", g.public(c.Name))
	}
	fmt.Fprintf(code, `)
		if err != nil {
			return nil, err
		}

		return &entity, nil
}
`)
}

func (g *Generator) generateRepositoryInsert(code *bytes.Buffer,
	table *pqt.Table) {
	entityName := g.private(table.Name)

	fmt.Fprintf(code, `func (r *%sRepositoryBase) Insert(e *%sEntity) (*%sEntity, error) {`, entityName, entityName, entityName)
	fmt.Fprintf(code, `
		insert := pqcomp.New(0, %d)
	`, len(table.Columns))

ColumnsLoop:
	for _, c := range table.Columns {
		switch c.Type {
		case pqt.TypeSerial(), pqt.TypeSerialBig(), pqt.TypeSerialSmall():
			continue ColumnsLoop
		default:
			if g.canBeNil(c, modeOptional) {
				fmt.Fprintf(code, `
					if e.%s != nil {
						insert.AddExpr(%s, "", e.%s)
					}
				`,
					g.public(c.Name),
					g.columnNameWithTableName(table.Name, c.Name), g.public(c.Name),
				)
			} else {
				fmt.Fprintf(code, `insert.AddExpr(%s, "", e.%s)`, g.columnNameWithTableName(table.Name, c.Name), g.public(c.Name))
			}
			fmt.Fprintln(code, "")
		}
	}
	fmt.Fprint(code, `
		b := bytes.NewBufferString("INSERT INTO " + r.table)

		if insert.Len() != 0 {
			b.WriteString(" (")
			for insert.Next() {
				if !insert.First() {
					b.WriteString(", ")
				}

				fmt.Fprintf(b, "%s", insert.Key())
			}
			insert.Reset()
			b.WriteString(") VALUES (")
			for insert.Next() {
				if !insert.First() {
					b.WriteString(", ")
				}

				fmt.Fprintf(b, "%s", insert.PlaceHolder())
			}
			b.WriteString(")")
			if len(r.columns) > 0 {
				b.WriteString("RETURNING ")
				b.WriteString(strings.Join(r.columns, ","))
			}
		}

		err := r.db.QueryRow(b.String(), insert.Args()...).Scan(
	`)

	for _, c := range table.Columns {
		fmt.Fprintf(code, "&e.%s,\n", g.public(c.Name))
	}
	fmt.Fprint(code, `)
		if err != nil {
			return nil, err
		}

		return e, nil
	}
`)
}

func (g *Generator) generateRepositoryUpdateByPrimaryKey(w io.Writer, table *pqt.Table) {
	entityName := g.private(table.Name)
	pk, ok := table.PrimaryKey()
	if !ok {
		return
	}

	fmt.Fprintf(w, "func (r *%sRepositoryBase) UpdateBy%s(patch *%sPatch) (*%sEntity, error) {\n", entityName, g.public(pk.Name), entityName, entityName)
	fmt.Fprintf(w, "update := pqcomp.New(0, %d)\n", len(table.Columns))

	if g.canBeNil(pk, modeOptional) {
		fmt.Fprintf(
			w,
			`
				if patch.%s != nil {
					update.AddExpr(%s, pqcomp.Equal, patch.%s)
				}
			`,
			g.private(pk.Name),
			g.columnNameWithTableName(table.Name, pk.Name),
			g.private(pk.Name),
		)
	} else {
		fmt.Fprintf(w, `update.AddExpr(%s, pqcomp.Equal, patch.%s)`, g.columnNameWithTableName(table.Name, pk.Name), g.private(pk.Name))
	}
	fmt.Fprintln(w, "")

ColumnsLoop:
	for _, c := range table.Columns {
		if c == pk {
			continue ColumnsLoop
		}
		if _, ok := c.DefaultOn(pqt.EventInsert, pqt.EventUpdate); ok {
			switch c.Type {
			case pqt.TypeTimestamp(), pqt.TypeTimestampTZ():
				fmt.Fprintf(w, "if patch.%s != nil {\n", g.private(c.Name))

			}
		}

		fmt.Fprintf(w, "update.AddExpr(")
		g.writeTableNameColumnNameTo(w, c.Table.Name, c.Name)
		fmt.Fprintf(w, ", pqcomp.Equal, patch.%s)\n", g.private(c.Name))

		if d, ok := c.DefaultOn(pqt.EventUpdate); ok {
			switch c.Type {
			case pqt.TypeTimestamp(), pqt.TypeTimestampTZ():
				fmt.Fprintf(w, `} else {`)
				fmt.Fprintf(w, "update.AddExpr(")
				g.writeTableNameColumnNameTo(w, c.Table.Name, c.Name)
				fmt.Fprintf(w, `, pqcomp.Equal, "%s")`, d)
			}
		}
		if _, ok := c.DefaultOn(pqt.EventInsert, pqt.EventUpdate); ok {
			switch c.Type {
			case pqt.TypeTimestamp(), pqt.TypeTimestampTZ():
				fmt.Fprintf(w, "\n}\n")
			}
		}
	}
	fmt.Fprintf(w, `
	if update.Len() == 0 {
		return nil, errors.New("%s: %s update failure, nothing to update")
	}`, g.pkg, entityName)

	fmt.Fprintf(w, `
	query := "UPDATE %s SET "
	for update.Next() {
		if !update.First() {
			query += ", "
		}

		query += update.Key() + " " + update.Oper() + " " + update.PlaceHolder()
	}
	query += " WHERE %s = $1 RETURNING " + strings.Join(r.columns, ", ")
	var e %sEntity
	err := r.db.QueryRow(query, update.Args()...).Scan(
	`, table.FullName(), pk.Name, entityName)
	for _, c := range table.Columns {
		fmt.Fprintf(w, "&e.%s,\n", g.public(c.Name))
	}
	fmt.Fprintf(w, `)
if err != nil {
	return nil, err
}


return &e, nil
}`)
}

func (g *Generator) generateRepositoryDeleteByPrimaryKey(code *bytes.Buffer,
	table *pqt.Table) {
	entityName := g.private(table.Name)
	pk, ok := table.PrimaryKey()
	if !ok {
		return
	}

	fmt.Fprintf(code, `
		func (r *%sRepositoryBase) DeleteBy%s(%s %s) (int64, error) {
			query := "DELETE FROM %s WHERE %s = $1"

			res, err := r.db.Exec(query, %s)
			if err != nil {
				return 0, err
			}

			return res.RowsAffected()
		}
	`, entityName, g.public(pk.Name), g.private(pk.Name), g.generateColumnTypeString(pk, 1), table.FullName(), pk.Name, g.private(pk.Name))
}

func sortedColumns(columns []*pqt.Column) []string {
	tmp := make([]string, 0, len(columns))
	for _, c := range columns {
		tmp = append(tmp, c.Name)
	}
	sort.Strings(tmp)

	return tmp
}

func snake(s string, private bool, acronyms map[string]string) string {
	var parts []string
	parts1 := strings.Split(s, "_")
	for _, p1 := range parts1 {
		parts2 := strings.Split(p1, "/")
		for _, p2 := range parts2 {
			parts3 := strings.Split(p2, "-")
			parts = append(parts, parts3...)
		}
	}

	for i, part := range parts {
		if !private || i > 0 {
			if formatted, ok := acronyms[part]; ok {
				parts[i] = formatted

				continue
			}
		}

		parts[i] = xstrings.FirstRuneToUpper(part)
	}

	if private {
		parts[0] = xstrings.FirstRuneToLower(parts[0])
	}

	return strings.Join(parts, "")
}

func (g *Generator) private(s string) string {
	return snake(s, true, g.acronyms)
}

func (g *Generator) public(s string) string {
	return snake(s, false, g.acronyms)
}

func (g *Generator) isStruct(c *pqt.Column, m int) bool {
	if tp, ok := c.Type.(pqt.MappableType); ok {
		for _, mapto := range tp.Mapping {
			if ct, ok := mapto.(CustomType); ok {
				switch m {
				case modeMandatory:
					return ct.mandatoryTypeOf.Kind() == reflect.Struct
				case modeOptional:
					return ct.optionalTypeOf.Kind() == reflect.Struct
				case modeCriteria:
					return ct.criteriaTypeOf.Kind() == reflect.Struct
				default:
					return false
				}
			}
		}
	}
	return false
}

func (g *Generator) canBeNil(c *pqt.Column, m int) bool {
	if tp, ok := c.Type.(pqt.MappableType); ok {
		for _, mapto := range tp.Mapping {
			if ct, ok := mapto.(CustomType); ok {
				switch m {
				case modeMandatory:
					return ct.mandatoryTypeOf.Kind() == reflect.Ptr || ct.mandatoryTypeOf.Kind() == reflect.Map
				case modeOptional:
					return ct.optionalTypeOf.Kind() == reflect.Ptr || ct.optionalTypeOf.Kind() == reflect.Map
				case modeCriteria:
					return ct.criteriaTypeOf.Kind() == reflect.Ptr || ct.criteriaTypeOf.Kind() == reflect.Map
				default:
					return false
				}
			}
		}
	}
	return false
}

func chooseType(tm, to, tc string, mode int32) string {
	switch mode {
	case modeCriteria:
		return tc
	case modeMandatory:
		return tm
	case modeOptional:
		return to
	case modeDefault:
		return to
	default:
		panic("unknown mode")
	}
}

func tableConstraints(t *pqt.Table) []*pqt.Constraint {
	var constraints []*pqt.Constraint
	for _, c := range t.Columns {
		constraints = append(constraints, c.Constraints()...)
	}

	return append(constraints, t.Constraints...)
}

func generateBaseType(t pqt.Type, m int32) string {
	switch t {
	case pqt.TypeText():
		return chooseType("string", "*ntypes.String", "*qtypes.String", m)
	case pqt.TypeBool():
		return chooseType("bool", "*ntypes.Bool", "*ntypes.Bool", m)
	case pqt.TypeIntegerSmall():
		return chooseType("int16", "*int16", "*int16", m)
	case pqt.TypeInteger():
		return chooseType("int32", "*ntypes.Int32", "*ntypes.Int32", m)
	case pqt.TypeIntegerBig():
		return chooseType("int64", "*ntypes.Int64", "*qtypes.Int64", m)
	case pqt.TypeSerial():
		return chooseType("int32", "*ntypes.Int32", "*ntypes.Int32", m)
	case pqt.TypeSerialSmall():
		return chooseType("int16", "*int16", "*int16", m)
	case pqt.TypeSerialBig():
		return chooseType("int64", "*ntypes.Int64", "*qtypes.Int64", m)
	case pqt.TypeTimestamp(), pqt.TypeTimestampTZ():
		return chooseType("time.Time", "*time.Time", "*qtypes.Timestamp", m)
	case pqt.TypeReal():
		return chooseType("float32", "*ntypes.Float32", "*ntypes.Float32", m)
	case pqt.TypeDoublePrecision():
		return chooseType("float64", "*ntypes.Float64", "*qtypes.Float64", m)
	case pqt.TypeBytea():
		return "[]byte"
	case pqt.TypeUUID():
		return "uuid.UUID"
	default:
		gt := t.String()
		switch {
		case strings.HasPrefix(gt, "SMALLINT["):
			return chooseType("pqt.ArrayInt64", "pqt.ArrayInt64", "*qtypes.Int64", m)
		case strings.HasPrefix(gt, "INTEGER["):
			return chooseType("pqt.ArrayInt64", "pqt.ArrayInt64", "*qtypes.Int64", m)
		case strings.HasPrefix(gt, "BIGINT["):
			return chooseType("pqt.ArrayInt64", "pqt.ArrayInt64", "*qtypes.Int64", m)
		case strings.HasPrefix(gt, "DOUBLE PRECISION["):
			return chooseType("pqt.ArrayFloat64", "pqt.ArrayFloat64", "*qtypes.Float64", m)
		case strings.HasPrefix(gt, "TEXT["):
			return "pqt.ArrayString"
		case strings.HasPrefix(gt, "DECIMAL"), strings.HasPrefix(gt, "NUMERIC"):
			return chooseType("float64", "*ntypes.Float64", "*qtypes.Float64", m)
		case strings.HasPrefix(gt, "VARCHAR"):
			return chooseType("string", "*ntypes.String", "*qtypes.String", m)
		default:
			return "interface{}"
		}
	}
}

func generateBuiltinType(t BuiltinType, m int32) (r string) {
	switch types.BasicKind(t) {
	case types.Bool:
		r = chooseType("bool", "*ntypes.Bool", "*ntypes.Bool", m)
	case types.Int:
		r = chooseType("int", "*ntypes.Int", "*ntypes.Int", m)
	case types.Int8:
		r = chooseType("int8", "*int8", "*int8", m)
	case types.Int16:
		r = chooseType("int16", "*int16", "*int16", m)
	case types.Int32:
		r = chooseType("int32", "*ntypes.Int32", "*ntypes.Int32", m)
	case types.Int64:
		r = chooseType("int64", "*ntypes.Int64", "*qtypes.Int64", m)
	case types.Uint:
		r = chooseType("uint", "*uint", "*uint", m)
	case types.Uint8:
		r = chooseType("uint8", "*uint8", "*uint8", m)
	case types.Uint16:
		r = chooseType("uint16", "*uint16", "*uint16", m)
	case types.Uint32:
		r = chooseType("uint32", "*ntypes.Uint32", "*ntypes.Uint32", m)
	case types.Uint64:
		r = chooseType("uint64", "*uint64", "*uint64", m)
	case types.Float32:
		r = chooseType("float32", "*ntypes.Float32", "*ntypes.Float32", m)
	case types.Float64:
		r = chooseType("float64", "*ntypes.Float64", "*qtypes.Float64", m)
	case types.Complex64:
		r = chooseType("complex64", "*complex64", "*complex64", m)
	case types.Complex128:
		r = chooseType("complex128", "*complex128", "*complex128", m)
	case types.String:
		r = chooseType("string", "*ntypes.String", "*qtypes.String", m)
	default:
		r = "invalid"
	}

	return
}

func generateCustomType(t CustomType, m int32) string {
	goType := func(tp reflect.Type) string {
		if tp.Kind() == reflect.Struct {
			return "*" + tp.String()
		}
		return tp.String()
	}
	return chooseType(
		goType(t.mandatoryTypeOf),
		goType(t.optionalTypeOf),
		goType(t.criteriaTypeOf),
		m,
	)
}

func (g *Generator) writeTableNameColumnNameTo(w io.Writer, tableName, columnName string) {
	fmt.Fprintf(w, "table%sColumn%s", g.public(tableName), g.public(columnName))
}

func (g *Generator) columnNameWithTableName(tableName, columnName string) string {
	return fmt.Sprintf("table%sColumn%s", g.public(tableName), g.public(columnName))
}

func (g *Generator) shouldBeColumnIgnoredForCriteria(c *pqt.Column) bool {
	return false
	//if mt, ok := c.Type.(pqt.MappableType); ok {
	//	switch mt.From {
	//	case pqt.TypeJSON(), pqt.TypeJSONB():
	//		for _, to := range mt.Mapping {
	//			if ct, ok := to.(*CustomType); ok {
	//				switch ct.valueOf.Kind() {
	//				case reflect.Struct:
	//					return false
	//				case reflect.Map:
	//					return false
	//				case reflect.Slice:
	//					return false
	//				case reflect.Slice:
	//					return false
	//				}
	//			}
	//		}
	//		return true
	//	}
	//}
	//
	//return false
}
