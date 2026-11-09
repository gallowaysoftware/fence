using System;
using System.Collections.Generic;
using System.Linq;
using System.Reflection;
using System.Runtime.CompilerServices;
using Fence.Compat;

namespace Fence.Mod
{
    // Reflection on the running game, for the hello compat report. Each lookup starts from a
    // literal typeof(...) in its own method: the game's ModScanner only accepts reflection whose
    // receiver it can trace to a game type, and a type removed by an update then fails as a
    // TypeLoadException inside that one method instead of breaking the whole class.
    sealed class RuntimeResolver : IMemberResolver
    {
        const BindingFlags All = BindingFlags.Public | BindingFlags.NonPublic | BindingFlags.Static | BindingFlags.Instance;

        public TypeLookup Lookup(string type, string member)
        {
            MemberInfo[] found;
            switch (type)
            {
                case "IMod": found = IModMembers(member); break;
                case "ModManifest": found = ModManifestMembers(member); break;
                case "ModLog": found = ModLogMembers(member); break;
                case "ModHelper": found = ModHelperMembers(member); break;
                case "ModConfig": found = ModConfigMembers(member); break;
                case "ConfigEntry`1": found = ConfigEntryMembers(member); break;
                case "ModHook": found = ModHookMembers(member); break;
                case "StoreStation": found = StoreStationMembers(member); break;
                case "AppraisalHelper": found = AppraisalHelperMembers(member); break;
                case "BargainUIManager": found = BargainUIManagerMembers(member); break;
                case "CustomUIBuilder": found = CustomUIBuilderMembers(member); break;
                case "CustomUIElement": found = CustomUIElementMembers(member); break;
                case "CustomUIManager": found = CustomUIManagerMembers(member); break;
                case "CustomUIWindow": found = CustomUIWindowMembers(member); break;
                case "ContrabandHelper": found = ContrabandHelperMembers(member); break;
                case "DialogUIManager": found = DialogUIManagerMembers(member); break;
                case "EmporiumEntry": found = EmporiumEntryMembers(member); break;
                case "GameItem": found = GameItemMembers(member); break;
                case "GeneralHelper": found = GeneralHelperMembers(member); break;
                case "ItemFeature": found = ItemFeatureMembers(member); break;
                case "MapUIManager": found = MapUIManagerMembers(member); break;
                case "NegociationData": found = NegociationDataMembers(member); break;
                case "NegociationUIManager": found = NegociationUIManagerMembers(member); break;
                case "PlayerStore": found = PlayerStoreMembers(member); break;
                case "RichTextBuilder": found = RichTextBuilderMembers(member); break;
                case "StartingPerk": found = StartingPerkMembers(member); break;
                case "StolenHelper": found = StolenHelperMembers(member); break;
                case "StoreClient": found = StoreClientMembers(member); break;
                case "StoreClientInstance": found = StoreClientInstanceMembers(member); break;
                case "StoreClientManager": found = StoreClientManagerMembers(member); break;
                case "StoreClientMono": found = StoreClientMonoMembers(member); break;
                case "StoreEvent": found = StoreEventMembers(member); break;
                case "StoreEventManager": found = StoreEventManagerMembers(member); break;
                case "StoreReputation": found = StoreReputationMembers(member); break;
                case "StoreUIManager": found = StoreUIManagerMembers(member); break;
                case "TagState": found = TagStateMembers(member); break;
                case "TypeHelper": found = TypeHelperMembers(member); break;
                case "AfterhourEventDefinition": found = AfterhourEventDefinitionMembers(member); break;
                case "AfterhourEvents": found = AfterhourEventsMembers(member); break;
                case "AfterhourLocation": found = AfterhourLocationMembers(member); break;
                case "AppraisalServiceHelper": found = AppraisalServiceHelperMembers(member); break;
                case "CommissaryData": found = CommissaryDataMembers(member); break;
                case "CommissaryUIManager": found = CommissaryUIManagerMembers(member); break;
                case "Dialogue": found = DialogueMembers(member); break;
                case "DialoguePhoneUIManager": found = DialoguePhoneUIManagerMembers(member); break;
                case "Difficulty": found = DifficultyMembers(member); break;
                case "DifficultyRules": found = DifficultyRulesMembers(member); break;
                case "EmporiumAnalytics": found = EmporiumAnalyticsMembers(member); break;
                case "EndOfDayUIManager": found = EndOfDayUIManagerMembers(member); break;
                case "GameInventory": found = GameInventoryMembers(member); break;
                case "GameOverUIManager": found = GameOverUIManagerMembers(member); break;
                case "GunOrderData": found = GunOrderDataMembers(member); break;
                case "HealthData": found = HealthDataMembers(member); break;
                case "InspectionUIManager": found = InspectionUIManagerMembers(member); break;
                case "ItemFeatureUI": found = ItemFeatureUIMembers(member); break;
                case "NegociationElement": found = NegociationElementMembers(member); break;
                case "NetworkUpgrade": found = NetworkUpgradeMembers(member); break;
                case "NotebookElement": found = NotebookElementMembers(member); break;
                case "RTSController": found = RTSControllerMembers(member); break;
                case "RunStats": found = RunStatsMembers(member); break;
                case "ScavHelper": found = ScavHelperMembers(member); break;
                case "SecData": found = SecDataMembers(member); break;
                case "ShowcaseHelper": found = ShowcaseHelperMembers(member); break;
                case "StoreBell": found = StoreBellMembers(member); break;
                case "StoreClientList": found = StoreClientListMembers(member); break;
                case "StoreClientListDict": found = StoreClientListDictMembers(member); break;
                case "StorePoster": found = StorePosterMembers(member); break;
                case "StoreReputationPerk": found = StoreReputationPerkMembers(member); break;
                case "StoreRequest": found = StoreRequestMembers(member); break;
                case "StoreService": found = StoreServiceMembers(member); break;
                // A guard type: reflecting on it is a scanner hard refusal, so it stays unchecked at runtime
                // (Fence.Compat still checks it offline).
                case "ModReflect": return TypeLookup.Unsupported();
                default: return TypeLookup.Unsupported();
            }
            return new TypeLookup { TypeFound = true, Members = found.Select(Describe).Where(m => m != null).ToList() };
        }

        [MethodImpl(MethodImplOptions.NoInlining)] static MemberInfo[] IModMembers(string m) => typeof(IMod).GetMember(m, All);
        [MethodImpl(MethodImplOptions.NoInlining)] static MemberInfo[] ModManifestMembers(string m) => typeof(ModManifest).GetMember(m, All);
        [MethodImpl(MethodImplOptions.NoInlining)] static MemberInfo[] ModLogMembers(string m) => typeof(ModLog).GetMember(m, All);
        [MethodImpl(MethodImplOptions.NoInlining)] static MemberInfo[] ModHelperMembers(string m) => typeof(ModHelper).GetMember(m, All);
        [MethodImpl(MethodImplOptions.NoInlining)] static MemberInfo[] ModConfigMembers(string m) => typeof(ModConfig).GetMember(m, All);
        [MethodImpl(MethodImplOptions.NoInlining)] static MemberInfo[] ConfigEntryMembers(string m) => typeof(ConfigEntry<>).GetMember(m, All);
        [MethodImpl(MethodImplOptions.NoInlining)] static MemberInfo[] ModHookMembers(string m) => typeof(ModHook).GetMember(m, All);
        [MethodImpl(MethodImplOptions.NoInlining)] static MemberInfo[] StoreStationMembers(string m) => typeof(StoreStation).GetMember(m, All);
        [MethodImpl(MethodImplOptions.NoInlining)] static MemberInfo[] AppraisalHelperMembers(string m) => typeof(AppraisalHelper).GetMember(m, All);
        [MethodImpl(MethodImplOptions.NoInlining)] static MemberInfo[] BargainUIManagerMembers(string m) => typeof(BargainUIManager).GetMember(m, All);
        [MethodImpl(MethodImplOptions.NoInlining)] static MemberInfo[] CustomUIBuilderMembers(string m) => typeof(CustomUIBuilder).GetMember(m, All);
        [MethodImpl(MethodImplOptions.NoInlining)] static MemberInfo[] CustomUIElementMembers(string m) => typeof(CustomUIElement).GetMember(m, All);
        [MethodImpl(MethodImplOptions.NoInlining)] static MemberInfo[] CustomUIManagerMembers(string m) => typeof(CustomUIManager).GetMember(m, All);
        [MethodImpl(MethodImplOptions.NoInlining)] static MemberInfo[] CustomUIWindowMembers(string m) => typeof(CustomUIWindow).GetMember(m, All);
        [MethodImpl(MethodImplOptions.NoInlining)] static MemberInfo[] ContrabandHelperMembers(string m) => typeof(ContrabandHelper).GetMember(m, All);
        [MethodImpl(MethodImplOptions.NoInlining)] static MemberInfo[] DialogUIManagerMembers(string m) => typeof(DialogUIManager).GetMember(m, All);
        [MethodImpl(MethodImplOptions.NoInlining)] static MemberInfo[] EmporiumEntryMembers(string m) => typeof(EmporiumEntry).GetMember(m, All);
        [MethodImpl(MethodImplOptions.NoInlining)] static MemberInfo[] GameItemMembers(string m) => typeof(GameItem).GetMember(m, All);
        [MethodImpl(MethodImplOptions.NoInlining)] static MemberInfo[] GeneralHelperMembers(string m) => typeof(GeneralHelper).GetMember(m, All);
        [MethodImpl(MethodImplOptions.NoInlining)] static MemberInfo[] ItemFeatureMembers(string m) => typeof(ItemFeature).GetMember(m, All);
        [MethodImpl(MethodImplOptions.NoInlining)] static MemberInfo[] MapUIManagerMembers(string m) => typeof(MapUIManager).GetMember(m, All);
        [MethodImpl(MethodImplOptions.NoInlining)] static MemberInfo[] NegociationDataMembers(string m) => typeof(NegociationData).GetMember(m, All);
        [MethodImpl(MethodImplOptions.NoInlining)] static MemberInfo[] NegociationUIManagerMembers(string m) => typeof(NegociationUIManager).GetMember(m, All);
        [MethodImpl(MethodImplOptions.NoInlining)] static MemberInfo[] PlayerStoreMembers(string m) => typeof(PlayerStore).GetMember(m, All);
        [MethodImpl(MethodImplOptions.NoInlining)] static MemberInfo[] RichTextBuilderMembers(string m) => typeof(RichTextBuilder).GetMember(m, All);
        [MethodImpl(MethodImplOptions.NoInlining)] static MemberInfo[] StartingPerkMembers(string m) => typeof(StartingPerk).GetMember(m, All);
        [MethodImpl(MethodImplOptions.NoInlining)] static MemberInfo[] StolenHelperMembers(string m) => typeof(StolenHelper).GetMember(m, All);
        [MethodImpl(MethodImplOptions.NoInlining)] static MemberInfo[] StoreClientMembers(string m) => typeof(StoreClient).GetMember(m, All);
        [MethodImpl(MethodImplOptions.NoInlining)] static MemberInfo[] StoreClientInstanceMembers(string m) => typeof(StoreClientInstance).GetMember(m, All);
        [MethodImpl(MethodImplOptions.NoInlining)] static MemberInfo[] StoreClientManagerMembers(string m) => typeof(StoreClientManager).GetMember(m, All);
        [MethodImpl(MethodImplOptions.NoInlining)] static MemberInfo[] StoreClientMonoMembers(string m) => typeof(StoreClientMono).GetMember(m, All);
        [MethodImpl(MethodImplOptions.NoInlining)] static MemberInfo[] StoreEventMembers(string m) => typeof(StoreEvent).GetMember(m, All);
        [MethodImpl(MethodImplOptions.NoInlining)] static MemberInfo[] StoreEventManagerMembers(string m) => typeof(StoreEventManager).GetMember(m, All);
        [MethodImpl(MethodImplOptions.NoInlining)] static MemberInfo[] StoreReputationMembers(string m) => typeof(StoreReputation).GetMember(m, All);
        [MethodImpl(MethodImplOptions.NoInlining)] static MemberInfo[] StoreUIManagerMembers(string m) => typeof(StoreUIManager).GetMember(m, All);
        [MethodImpl(MethodImplOptions.NoInlining)] static MemberInfo[] TagStateMembers(string m) => typeof(TagState).GetMember(m, All);
        [MethodImpl(MethodImplOptions.NoInlining)] static MemberInfo[] TypeHelperMembers(string m) => typeof(TypeHelper).GetMember(m, All);
        [MethodImpl(MethodImplOptions.NoInlining)] static MemberInfo[] AfterhourEventDefinitionMembers(string m) => typeof(AfterhourEventDefinition).GetMember(m, All);
        [MethodImpl(MethodImplOptions.NoInlining)] static MemberInfo[] AfterhourEventsMembers(string m) => typeof(AfterhourEvents).GetMember(m, All);
        [MethodImpl(MethodImplOptions.NoInlining)] static MemberInfo[] AfterhourLocationMembers(string m) => typeof(AfterhourLocation).GetMember(m, All);
        [MethodImpl(MethodImplOptions.NoInlining)] static MemberInfo[] AppraisalServiceHelperMembers(string m) => typeof(AppraisalServiceHelper).GetMember(m, All);
        [MethodImpl(MethodImplOptions.NoInlining)] static MemberInfo[] CommissaryDataMembers(string m) => typeof(CommissaryData).GetMember(m, All);
        [MethodImpl(MethodImplOptions.NoInlining)] static MemberInfo[] CommissaryUIManagerMembers(string m) => typeof(CommissaryUIManager).GetMember(m, All);
        [MethodImpl(MethodImplOptions.NoInlining)] static MemberInfo[] DialogueMembers(string m) => typeof(Dialogue).GetMember(m, All);
        [MethodImpl(MethodImplOptions.NoInlining)] static MemberInfo[] DialoguePhoneUIManagerMembers(string m) => typeof(DialoguePhoneUIManager).GetMember(m, All);
        [MethodImpl(MethodImplOptions.NoInlining)] static MemberInfo[] DifficultyMembers(string m) => typeof(Difficulty).GetMember(m, All);
        [MethodImpl(MethodImplOptions.NoInlining)] static MemberInfo[] DifficultyRulesMembers(string m) => typeof(DifficultyRules).GetMember(m, All);
        [MethodImpl(MethodImplOptions.NoInlining)] static MemberInfo[] EmporiumAnalyticsMembers(string m) => typeof(EmporiumAnalytics).GetMember(m, All);
        [MethodImpl(MethodImplOptions.NoInlining)] static MemberInfo[] EndOfDayUIManagerMembers(string m) => typeof(EndOfDayUIManager).GetMember(m, All);
        [MethodImpl(MethodImplOptions.NoInlining)] static MemberInfo[] GameInventoryMembers(string m) => typeof(GameInventory).GetMember(m, All);
        [MethodImpl(MethodImplOptions.NoInlining)] static MemberInfo[] GameOverUIManagerMembers(string m) => typeof(GameOverUIManager).GetMember(m, All);
        [MethodImpl(MethodImplOptions.NoInlining)] static MemberInfo[] GunOrderDataMembers(string m) => typeof(GunOrderData).GetMember(m, All);
        [MethodImpl(MethodImplOptions.NoInlining)] static MemberInfo[] HealthDataMembers(string m) => typeof(HealthData).GetMember(m, All);
        [MethodImpl(MethodImplOptions.NoInlining)] static MemberInfo[] InspectionUIManagerMembers(string m) => typeof(InspectionUIManager).GetMember(m, All);
        [MethodImpl(MethodImplOptions.NoInlining)] static MemberInfo[] ItemFeatureUIMembers(string m) => typeof(ItemFeatureUI).GetMember(m, All);
        [MethodImpl(MethodImplOptions.NoInlining)] static MemberInfo[] NegociationElementMembers(string m) => typeof(NegociationElement).GetMember(m, All);
        [MethodImpl(MethodImplOptions.NoInlining)] static MemberInfo[] NetworkUpgradeMembers(string m) => typeof(NetworkUpgrade).GetMember(m, All);
        [MethodImpl(MethodImplOptions.NoInlining)] static MemberInfo[] NotebookElementMembers(string m) => typeof(NotebookElement).GetMember(m, All);
        [MethodImpl(MethodImplOptions.NoInlining)] static MemberInfo[] RTSControllerMembers(string m) => typeof(RTSController).GetMember(m, All);
        [MethodImpl(MethodImplOptions.NoInlining)] static MemberInfo[] RunStatsMembers(string m) => typeof(RunStats).GetMember(m, All);
        [MethodImpl(MethodImplOptions.NoInlining)] static MemberInfo[] ScavHelperMembers(string m) => typeof(ScavHelper).GetMember(m, All);
        [MethodImpl(MethodImplOptions.NoInlining)] static MemberInfo[] SecDataMembers(string m) => typeof(SecData).GetMember(m, All);
        [MethodImpl(MethodImplOptions.NoInlining)] static MemberInfo[] ShowcaseHelperMembers(string m) => typeof(ShowcaseHelper).GetMember(m, All);
        [MethodImpl(MethodImplOptions.NoInlining)] static MemberInfo[] StoreBellMembers(string m) => typeof(StoreBell).GetMember(m, All);
        [MethodImpl(MethodImplOptions.NoInlining)] static MemberInfo[] StoreClientListMembers(string m) => typeof(StoreClientList).GetMember(m, All);
        [MethodImpl(MethodImplOptions.NoInlining)] static MemberInfo[] StoreClientListDictMembers(string m) => typeof(StoreClientListDict).GetMember(m, All);
        [MethodImpl(MethodImplOptions.NoInlining)] static MemberInfo[] StorePosterMembers(string m) => typeof(StorePoster).GetMember(m, All);
        [MethodImpl(MethodImplOptions.NoInlining)] static MemberInfo[] StoreReputationPerkMembers(string m) => typeof(StoreReputationPerk).GetMember(m, All);
        [MethodImpl(MethodImplOptions.NoInlining)] static MemberInfo[] StoreRequestMembers(string m) => typeof(StoreRequest).GetMember(m, All);
        [MethodImpl(MethodImplOptions.NoInlining)] static MemberInfo[] StoreServiceMembers(string m) => typeof(StoreService).GetMember(m, All);

        static MemberSignature Describe(MemberInfo m)
        {
            switch (m)
            {
                case EventInfo e:
                    return new MemberSignature { Kind = MemberKinds.Event, Returns = Format(e.EventHandlerType) };
                case MethodInfo mi:
                    return new MemberSignature { Kind = MemberKinds.Method, Params = Params(mi), Returns = Format(mi.ReturnType) };
                case ConstructorInfo ci:
                    return new MemberSignature { Kind = MemberKinds.Method, Params = Params(ci), Returns = "System.Void" };
                case FieldInfo f:
                    return new MemberSignature { Kind = MemberKinds.Field, Returns = Format(f.FieldType) };
                case PropertyInfo p:
                    return new MemberSignature { Kind = MemberKinds.Property, Returns = Format(p.PropertyType) };
                default:
                    return null;
            }
        }

        static List<string> Params(MethodBase m) => m.GetParameters().Select(p => Format(p.ParameterType)).ToList();

        static string Format(Type t)
        {
            if (t.IsByRef) return Format(t.GetElementType()) + "&";
            if (t.IsArray) return Format(t.GetElementType()) + "[" + new string(',', t.GetArrayRank() - 1) + "]";
            if (t.IsGenericParameter) return t.Name;
            var args = t.IsGenericType ? t.GetGenericArguments().Select(Format).ToList() : null;
            return TypeNames.Compose(t.Namespace, t.Name, t.IsNested ? Format(t.DeclaringType) : null, args);
        }
    }
}
