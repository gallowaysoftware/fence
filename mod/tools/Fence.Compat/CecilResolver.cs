using System.Collections.Generic;
using System.Linq;
using Fence.Compat;
using Mono.Cecil;

namespace Fence.CompatTool
{
    public static class CecilNames
    {
        public static string Format(TypeReference t)
        {
            switch (t)
            {
                case IModifierType mod:
                    return Format(mod.ElementType);
                case ByReferenceType br:
                    return Format(br.ElementType) + "&";
                case PointerType p:
                    return Format(p.ElementType) + "*";
                case ArrayType a:
                    return Format(a.ElementType) + "[" + new string(',', a.Rank - 1) + "]";
                case GenericParameter gp:
                    return gp.Name;
                case GenericInstanceType gi:
                    return Compose(gi.ElementType, gi.GenericArguments.Select(Format).ToList());
                default:
                    // A generic definition spelled like reflection spells it: ConfigEntry<T>.
                    var args = t.HasGenericParameters ? t.GenericParameters.Select(g => g.Name).ToList() : null;
                    return Compose(t, args);
            }
        }

        static string Compose(TypeReference t, List<string> args) =>
            TypeNames.Compose(t.Namespace, t.Name, t.DeclaringType != null ? Format(t.DeclaringType) : null, args);
    }

    // Offline counterpart of the mod's RuntimeResolver: same lookup semantics (public and non-public,
    // static and instance, inherited from game base types) over Assembly-CSharp read by Cecil.
    public sealed class CecilResolver : IMemberResolver
    {
        readonly ModuleDefinition module;

        public CecilResolver(ModuleDefinition module)
        {
            this.module = module;
        }

        public TypeDefinition FindType(string name) => module.GetType(name.Replace('+', '/'));

        public TypeLookup Lookup(string type, string member)
        {
            var def = FindType(type);
            if (def == null) return TypeLookup.MissingType();
            var lookup = new TypeLookup { TypeFound = true };
            for (var t = def; t != null; t = BaseInModule(t))
            {
                foreach (var e in t.Events.Where(e => e.Name == member))
                    lookup.Members.Add(new MemberSignature { Kind = MemberKinds.Event, Returns = CecilNames.Format(e.EventType) });
                foreach (var m in t.Methods.Where(m => m.Name == member))
                    lookup.Members.Add(new MemberSignature
                    {
                        Kind = MemberKinds.Method,
                        Params = m.Parameters.Select(p => CecilNames.Format(p.ParameterType)).ToList(),
                        Returns = m.IsConstructor ? "System.Void" : CecilNames.Format(m.ReturnType),
                    });
                foreach (var f in t.Fields.Where(f => f.Name == member))
                    lookup.Members.Add(new MemberSignature { Kind = MemberKinds.Field, Returns = CecilNames.Format(f.FieldType) });
                foreach (var p in t.Properties.Where(p => p.Name == member))
                    lookup.Members.Add(new MemberSignature { Kind = MemberKinds.Property, Returns = CecilNames.Format(p.PropertyType) });
                if (member == ".ctor") break;
            }
            return lookup;
        }

        TypeDefinition BaseInModule(TypeDefinition t)
        {
            var b = t.BaseType;
            if (b == null || b.Scope != module && b.Scope?.Name != module.Assembly.Name.Name) return null;
            return b.Resolve();
        }
    }
}
