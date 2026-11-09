using System;
using System.Collections.Generic;
using System.Linq;
using System.Text;
using Mono.Cecil;

namespace Fence.CompatTool
{
    // Public and protected signatures only, one per line, sorted: a stable text to `git diff`
    // between game builds. Bodies and private members are deliberately left out.
    public static class SurfaceDump
    {
        public static string Render(CecilResolver resolver, IEnumerable<string> typeNames)
        {
            var sb = new StringBuilder();
            foreach (var name in typeNames.Distinct().OrderBy(n => n, StringComparer.Ordinal))
            {
                var t = resolver.FindType(name);
                if (t == null)
                {
                    sb.Append("type ").Append(name).Append(" MISSING\n\n");
                    continue;
                }
                sb.Append("type ").Append(Kind(t)).Append(' ').Append(CecilNames.Format(t));
                if (t.BaseType != null && t.BaseType.FullName != "System.Object") sb.Append(" : ").Append(CecilNames.Format(t.BaseType));
                sb.Append('\n');
                foreach (var line in Members(t).OrderBy(l => l, StringComparer.Ordinal)) sb.Append("  ").Append(line).Append('\n');
                sb.Append('\n');
            }
            return sb.ToString();
        }

        static string Kind(TypeDefinition t) =>
            t.IsInterface ? "interface" : t.IsEnum ? "enum" : t.IsValueType ? "struct" : t.IsAbstract && t.IsSealed ? "static class" : "class";

        static IEnumerable<string> Members(TypeDefinition t)
        {
            foreach (var f in t.Fields.Where(f => f.IsPublic || f.IsFamily || f.IsFamilyOrAssembly))
            {
                if (t.IsEnum && f.IsSpecialName) continue;
                yield return "field " + Vis(f.IsPublic) + (f.IsLiteral ? "const " : f.IsStatic ? "static " : "")
                             + (f.IsInitOnly ? "readonly " : "") + CecilNames.Format(f.FieldType) + " " + f.Name
                             + (f.HasConstant ? " = " + Literal(f.Constant) : "");
            }
            foreach (var m in t.Methods.Where(m => Visible(m) && !m.IsGetter && !m.IsSetter && !m.IsAddOn && !m.IsRemoveOn))
            {
                string ps = string.Join(", ", m.Parameters.Select(p => CecilNames.Format(p.ParameterType) + " " + p.Name));
                if (m.IsConstructor)
                {
                    if (m.IsStatic) continue;
                    yield return "ctor " + Vis(m.IsPublic) + "(" + ps + ")";
                }
                else
                {
                    string gen = m.HasGenericParameters ? "<" + string.Join(",", m.GenericParameters.Select(g => g.Name)) + ">" : "";
                    yield return "method " + Vis(m.IsPublic) + (m.IsStatic ? "static " : "") + (m.IsAbstract ? "abstract " : m.IsVirtual ? "virtual " : "")
                                 + CecilNames.Format(m.ReturnType) + " " + m.Name + gen + "(" + ps + ")";
                }
            }
            foreach (var p in t.Properties)
            {
                bool get = p.GetMethod != null && Visible(p.GetMethod);
                bool set = p.SetMethod != null && Visible(p.SetMethod);
                if (!get && !set) continue;
                var any = get ? p.GetMethod : p.SetMethod;
                yield return "property " + (any.IsStatic ? "static " : "") + CecilNames.Format(p.PropertyType) + " " + p.Name
                             + " {" + (get ? " get;" : "") + (set ? " set;" : "") + " }";
            }
            foreach (var e in t.Events.Where(e => e.AddMethod != null && Visible(e.AddMethod)))
                yield return "event " + (e.AddMethod.IsStatic ? "static " : "") + CecilNames.Format(e.EventType) + " " + e.Name;
        }

        static bool Visible(MethodDefinition m) => m.IsPublic || m.IsFamily || m.IsFamilyOrAssembly;

        static string Vis(bool isPublic) => isPublic ? "" : "protected ";

        static string Literal(object v) => v switch
        {
            null => "null",
            string s => "\"" + s + "\"",
            bool b => b ? "true" : "false",
            IFormattable f => f.ToString(null, System.Globalization.CultureInfo.InvariantCulture),
            _ => v.ToString(),
        };
    }
}
