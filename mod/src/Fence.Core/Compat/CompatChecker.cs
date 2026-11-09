using System;
using System.Collections.Generic;
using System.Linq;
using Fence.Protocol;

namespace Fence.Compat
{
    public sealed class MemberSignature
    {
        public string Kind;
        public List<string> Params = new List<string>();
        public string Returns;

        public override string ToString() => Kind == MemberKinds.Method
            ? Returns + " (" + string.Join(", ", Params) + ")"
            : Kind + " " + Returns;
    }

    public sealed class TypeLookup
    {
        public bool Supported = true;
        public bool TypeFound;
        public List<MemberSignature> Members = new List<MemberSignature>();

        public static TypeLookup Unsupported() => new TypeLookup { Supported = false };
        public static TypeLookup MissingType() => new TypeLookup { TypeFound = false };
    }

    // Implemented by reflection on the loaded game (runtime) and by Cecil on the DLL (offline), so
    // both sides judge a build by the same rules.
    public interface IMemberResolver
    {
        // Members of `type` named `member` (".ctor" for constructors), with signatures spelled by TypeNames.
        TypeLookup Lookup(string type, string member);
    }

    public enum HookStatus
    {
        Ok,
        Unchecked,
        MissingType,
        MissingMember,
        KindChanged,
        ParamsChanged,
        ReturnChanged,
        BindFailed,
    }

    public sealed class HookResult
    {
        public HookSpec Hook;
        public HookStatus Status;
        public string Detail;

        public bool Failed => Status != HookStatus.Ok && Status != HookStatus.Unchecked;
    }

    public sealed class CompatReport
    {
        public List<HookResult> Results = new List<HookResult>();

        public IEnumerable<HookResult> Failures => Results.Where(r => r.Failed);
        public bool Ok => !Failures.Any();
        public bool RequiredOk => !Failures.Any(r => r.Hook.Required);

        public HookResult For(string hookId) => Results.FirstOrDefault(r => r.Hook.Id == hookId);

        public bool IsUsable(string hookId)
        {
            var r = For(hookId);
            return r != null && !r.Failed;
        }

        // A hook that resolved but could not be bound at runtime (subscription threw, JIT failed).
        public void MarkBindFailed(string hookId, string detail)
        {
            var r = For(hookId);
            if (r == null || r.Failed) return;
            r.Status = HookStatus.BindFailed;
            r.Detail = detail;
        }

        public HashSet<string> DisabledFeatures() =>
            new HashSet<string>(Failures.SelectMany(r => r.Hook.Features ?? new List<string>()));

        public HelloCompat ToHello() => new HelloCompat
        {
            Ok = Ok,
            Missing = Failures.Select(r => r.Hook.Display).Distinct().OrderBy(s => s, StringComparer.Ordinal).ToList(),
            DisabledFeatures = DisabledFeatures().OrderBy(s => s, StringComparer.Ordinal).ToList(),
        };
    }

    public static class CompatChecker
    {
        public static CompatReport Check(HookManifest manifest, IMemberResolver resolver)
        {
            var report = new CompatReport();
            foreach (var hook in manifest.Hooks) report.Results.Add(CheckOne(hook, resolver));
            return report;
        }

        public static HookResult CheckOne(HookSpec hook, IMemberResolver resolver)
        {
            TypeLookup lookup;
            try
            {
                lookup = resolver.Lookup(hook.Type, hook.Member);
            }
            catch (Exception e)
            {
                return Result(hook, HookStatus.MissingType, e.GetType().Name + ": " + e.Message);
            }

            if (!lookup.Supported) return Result(hook, HookStatus.Unchecked, "resolver cannot inspect " + hook.Type);
            if (!lookup.TypeFound) return Result(hook, HookStatus.MissingType, "type " + hook.Type + " not found");
            if (lookup.Members.Count == 0) return Result(hook, HookStatus.MissingMember, hook.Display + " not found");

            var sameKind = lookup.Members.Where(m => m.Kind == hook.Kind).ToList();
            if (sameKind.Count == 0)
                return Result(hook, HookStatus.KindChanged, "is now " + string.Join(", ", lookup.Members.Select(m => m.Kind).Distinct()));

            if (hook.Kind == MemberKinds.Method)
            {
                var wanted = hook.Params ?? new List<string>();
                var overload = sameKind.FirstOrDefault(m => m.Params.SequenceEqual(wanted));
                if (overload == null)
                    return Result(hook, HookStatus.ParamsChanged, "overloads: " + string.Join(" | ", sameKind.Select(m => m.ToString())));
                if (overload.Returns != hook.Returns)
                    return Result(hook, HookStatus.ReturnChanged, "returns " + overload.Returns + ", expected " + hook.Returns);
                return Result(hook, HookStatus.Ok, null);
            }

            var member = sameKind[0];
            if (member.Returns != hook.Returns)
                return Result(hook, HookStatus.ReturnChanged, "is " + member.Returns + ", expected " + hook.Returns);
            return Result(hook, HookStatus.Ok, null);
        }

        static HookResult Result(HookSpec hook, HookStatus status, string detail) =>
            new HookResult { Hook = hook, Status = status, Detail = detail };
    }
}
