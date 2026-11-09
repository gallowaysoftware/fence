using System.Collections.Generic;

namespace Fence.Compat
{
    // Canonical type spelling shared by hooks.json, the runtime (System.Reflection) resolver and the
    // offline (Cecil) resolver: "System.Action<System.String,PaperUI>", "Outer+Inner", "T[]", "T&",
    // generic parameters by name. Global-namespace game types are bare.
    public static class TypeNames
    {
        public static string Compose(string ns, string name, string declaringType, IList<string> genericArgs)
        {
            string bare = StripArity(name);
            string head = declaringType != null
                ? declaringType + "+" + bare
                : string.IsNullOrEmpty(ns) ? bare : ns + "." + bare;
            if (genericArgs == null || genericArgs.Count == 0) return head;
            return head + "<" + string.Join(",", genericArgs) + ">";
        }

        public static string StripArity(string name)
        {
            int tick = name.IndexOf('`');
            return tick < 0 ? name : name.Substring(0, tick);
        }
    }
}
