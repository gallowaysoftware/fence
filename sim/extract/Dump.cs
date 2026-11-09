using System;
using System.Collections;
using System.Collections.Generic;
using System.Linq;
using System.Reflection;
using System.Runtime.CompilerServices;
using System.Text.Json;

using Fence.Sim;

namespace Fence.Extract
{
    // Reflection walk of game objects into JSON. Every instance field (public or private, auto-property
    // backing fields renamed) down to a depth limit; delegates become the name of the method they
    // point at, which for the game's factories is the most useful thing to know about them.
    sealed class Dump
    {
        readonly Utf8JsonWriter w;
        readonly int maxDepth;
        readonly HashSet<object> path = new HashSet<object>(ReferenceEqualityComparer.Instance);
        public Func<FieldInfo, bool> Skip = _ => false;

        public Dump(Utf8JsonWriter w, int maxDepth = 4)
        {
            this.w = w;
            this.maxDepth = maxDepth;
        }

        public static string FieldName(FieldInfo f)
        {
            string n = f.Name;
            if (n.StartsWith("<") && n.Contains(">k__BackingField")) n = n.Substring(1, n.IndexOf('>') - 1);
            return n;
        }

        public static string DelegateName(Delegate d)
        {
            var parts = d.GetInvocationList().Select(x => x.Method.DeclaringType?.Name + "." + x.Method.Name);
            return string.Join(" + ", parts);
        }

        public static IEnumerable<FieldInfo> Fields(Type t)
        {
            var chain = new List<Type>();
            for (var b = t; b != null && b != typeof(object); b = b.BaseType) chain.Add(b);
            chain.Reverse();
            foreach (var c in chain)
                foreach (var f in c.GetFields(BindingFlags.Instance | BindingFlags.Public | BindingFlags.NonPublic | BindingFlags.DeclaredOnly))
                    yield return f;
        }

        public void Value(object v, int depth = 0)
        {
            switch (v)
            {
                case null: w.WriteNullValue(); return;
                // Runtime GUIDs (Guid.NewGuid identifiers) would make every extraction differ.
                case string s: w.WriteStringValue(s.Length == 36 && Guid.TryParse(s, out _) ? "<guid>" : s); return;
                case bool b: w.WriteBooleanValue(b); return;
                case Enum e: w.WriteStringValue(e.ToString()); return;
                case int or long or short or byte or sbyte or ushort or uint or ulong:
                    w.WriteNumberValue(Convert.ToInt64(v)); return;
                case float f: w.WriteNumberValue(Math.Round((double)f, 6)); return;
                case double d:
                    if (double.IsFinite(d)) w.WriteNumberValue(d); else w.WriteStringValue(d.ToString());
                    return;
                case decimal m: w.WriteNumberValue(m); return;
                case Delegate del: w.WriteStringValue("fn:" + DelegateName(del)); return;
                case Type ty: w.WriteStringValue("type:" + ty.Name); return;
                case UnityEngine.Object uo: w.WriteStringValue("unity:" + uo.GetType().Name); return;
            }
            var type = v.GetType();
            if (type.Assembly != typeof(GameItem).Assembly && !(v is IEnumerable) && !type.IsValueType)
            {
                w.WriteStringValue(type.Name);
                return;
            }
            if (depth >= maxDepth || path.Contains(v))
            {
                w.WriteStringValue("…" + type.Name);
                return;
            }
            path.Add(v);
            try
            {
                if (v is IDictionary dict)
                {
                    w.WriteStartObject();
                    var keys = dict.Keys.Cast<object>().Select(k => (k, s: Convert.ToString(k))).OrderBy(k => k.s, StringComparer.Ordinal);
                    foreach (var (k, s) in keys)
                    {
                        w.WritePropertyName(s ?? "null");
                        Value(dict[k], depth + 1);
                    }
                    w.WriteEndObject();
                    return;
                }
                if (v is IEnumerable seq)
                {
                    w.WriteStartArray();
                    foreach (var x in seq) Value(x, depth + 1);
                    w.WriteEndArray();
                    return;
                }
                if (type.IsGenericType && type.FullName.StartsWith("System.ValueTuple"))
                {
                    w.WriteStartArray();
                    foreach (var f in type.GetFields()) Value(f.GetValue(v), depth + 1);
                    w.WriteEndArray();
                    return;
                }
                Object(v, depth);
            }
            finally
            {
                path.Remove(v);
            }
        }

        public void Object(object v, int depth = 0)
        {
            w.WriteStartObject();
            w.WriteString("$type", v.GetType().Name);
            foreach (var f in Fields(v.GetType()))
            {
                if (Skip(f) || f.FieldType.IsPointer) continue;
                object x;
                try { x = f.GetValue(v); } catch { continue; }
                if (IsEmpty(x)) continue;
                w.WritePropertyName(FieldName(f));
                Value(x, depth + 1);
            }
            w.WriteEndObject();
        }

        // Defaults are dropped to keep the tables readable; absent means default.
        static bool IsEmpty(object x) => x switch
        {
            null => true,
            bool b => !b,
            int i => i == 0,
            long l => l == 0,
            float f => f == 0,
            double d => d == 0,
            string s => s.Length == 0,
            ICollection c => c.Count == 0,
            _ => false,
        };
    }
}
