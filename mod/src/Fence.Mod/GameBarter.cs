using System;
using System.Collections.Generic;
using System.Runtime.CompilerServices;
using Fence.Barter;

namespace Fence.Mod
{
    // IBarterGame over the live game. Reads only; every member is listed in compat/hooks.json under
    // advise.barter. Each method is NoInlining so a member an update removed fails here, inside the
    // caller's try/catch, instead of breaking the class.
    sealed class GameBarter : IBarterGame
    {
        readonly Dictionary<GameItem, int> localIds = new Dictionary<GameItem, int>();

        public static ClientIntent MapIntent(StoreClient.ClientIntent i)
        {
            switch (i)
            {
                case StoreClient.ClientIntent.BUY: return ClientIntent.Buy;
                case StoreClient.ClientIntent.SELL: return ClientIntent.Sell;
                case StoreClient.ClientIntent.SELLNBUY: return ClientIntent.SellAndBuy;
                case StoreClient.ClientIntent.DIALOGUE: return ClientIntent.Dialogue;
                default: return ClientIntent.Other;
            }
        }

        [MethodImpl(MethodImplOptions.NoInlining)]
        public static StoreClient CurrentClient()
        {
            var store = PlayerStore.Instance;
            var instance = store != null ? store.currentClientInstance : null;
            return instance != null ? instance.GetClientBlueprint() : null;
        }

        [MethodImpl(MethodImplOptions.NoInlining)]
        public StoreInfo Store()
        {
            var store = PlayerStore.Instance;
            var info = new StoreInfo();
            if (store == null) return info;
            info.Cash = store.playerCash;
            info.RetailMarkup = store.retailMarkup;
            var station = StoreStation.Instance;
            if (station != null)
            {
                info.Day = station.dayCounter;
                info.RentDue = store.GetRentValue();
                info.RentDaysLeft = store.GetRentDayCounter();
            }
            return info;
        }

        [MethodImpl(MethodImplOptions.NoInlining)]
        public IList<FactionRep> Reputations()
        {
            var list = new List<FactionRep>();
            var store = PlayerStore.Instance;
            if (store == null || store.storeReputations == null) return list;
            foreach (var r in store.storeReputations)
            {
                if (r == null) continue;
                list.Add(new FactionRep
                {
                    FactionId = r.factionId,
                    Value = r.GetReputationExact(),
                    Status = StoreReputation.GetReputationStatus(r.GetReputation()),
                });
            }
            return list;
        }

        [MethodImpl(MethodImplOptions.NoInlining)]
        public ClientInfo Client()
        {
            var c = CurrentClient();
            if (c == null) return null;
            var info = new ClientInfo
            {
                Key = c,
                Name = c.displayName,
                Identifier = c.identifier,
                FactionId = c.clientFaction,
                Intent = MapIntent(c.clientIntent),
                RawIntent = c.clientIntent.ToString(),
                UsesBudget = c.useClientBudget,
                Budget = c.GetBudget(),
                NoContraband = c.noAcceptingContraband,
                RequiresVariety = c.isRequireVariety,
                IsSecurity = c.isSecurity,
                IsWanted = c.isWanted,
                IsStealthWanted = c.isStealthWanted,
                IsAug = c.isAug,
                AttractedItemId = c.attractedItemId,
                AttractedType = c.attractedType,
                OffersWholesale = c.isOfferWholesale,
                WholesaleDiscount = c.wholesaleDiscount,
                Dismissable = c.dismissable,
                BargainAvailable = c.isBargainAvailable,
                MainDialogueStarted = c.isMainDialogueStarted,
                BusinessComplete = c.isBusinessComplete,
                RegularBuyDisabled = c.isRegularSellDisable,
                MultiBuyDisabled = c.isMultiBuyDisabled,
            };
            // NegociationUIManager.BuildInfoBlock: only known item types become tags, and only for a buying client.
            if (BarterRules.IsBuying(info.Intent))
            {
                var known = TypeHelper.ALL_TYPES_LIST;
                if (c.clientBuyingTagList != null)
                    foreach (var t in c.clientBuyingTagList)
                        if (known.Contains(t)) info.InterestTags.Add(t == "ALL_TYPE_EXCEPT_CONTRABAND" ? "ALL_TYPE" : t);
                if (c.clientBlackTagList != null)
                    foreach (var t in c.clientBlackTagList)
                        if (known.Contains(t)) info.RestrictionTags.Add(t);
            }
            if (c.isOfferWholesale && PlayerStore.Instance != null) info.WholesalePrice = PlayerStore.Instance.GetWholesalePrice();
            if (!augBroken)
            {
                try
                {
                    ReadAug(c, info);
                }
                catch (Exception)
                {
                    // Members listed under advise.aug; without them the faction stays as read and aug tells are absent.
                    augBroken = true;
                }
            }
            return info;
        }

        bool augBroken;

        // The bubble and the aug tells (docs/re/AUGS.md §2): what the screen shows, plus hidden labels.
        [MethodImpl(MethodImplOptions.NoInlining)]
        static void ReadAug(StoreClient c, ClientInfo info)
        {
            info.BubbleColor = c.bubbleColor;
            var a = new Fence.Aug.AugInfo
            {
                Sprite = c.spriteName,
                Scanned = c.isAugScanned,
                Tier = c.augTier,
                IdleTell = c.augIdleTell.ToString().ToLowerInvariant(),
                Disguised = c.isAug && c.identifier != null && c.identifier.EndsWith("_disguised_aug", StringComparison.Ordinal),
            };
            info.Aug = a;
            try
            {
                a.Idle = Idle(c);
            }
            catch (Exception)
            {
            }
            try
            {
                string name = c.displayName;
                if (name == LocHelper.GetLocalizedClientName("name_upper_level_citizen")) a.NameClaim = "upper_levels";
                else if (name == LocHelper.GetLocalizedClientName("name_thief")) a.NameClaim = "black_market";
            }
            catch (Exception)
            {
            }
        }

        // The aug script's line templates from the installed game's string table (AugLines), once it has them.
        [MethodImpl(MethodImplOptions.NoInlining)]
        public static bool EnsureAugLines()
        {
            if (Fence.Aug.AugLines.Installed) return true;
            return Fence.Aug.AugLines.Install(key => LocHelper.GetLocalizedDialogue(key, Fence.Aug.AugLines.Slot)) > 0;
        }

        // StoreClientMono's private idle flags, valid only once it decided them for this customer.
        [MethodImpl(MethodImplOptions.NoInlining)]
        static string Idle(StoreClient c)
        {
            var mono = StoreClientMono.Instance;
            if (mono == null || !ReferenceEquals(ModReflect.Get(mono, "idleDecisionClient"), c)) return null;
            if ((bool)ModReflect.Get(mono, "idleSuppressed")) return Fence.Aug.AugIdle.Still;
            if ((bool)ModReflect.Get(mono, "idleShakeHorizontal")) return Fence.Aug.AugIdle.Twitch;
            return Fence.Aug.AugIdle.Normal;
        }

        [MethodImpl(MethodImplOptions.NoInlining)]
        public IList<ItemInfo> CounterItems()
        {
            var list = new List<ItemInfo>();
            var entry = EmporiumEntry.Instance;
            var client = CurrentClient();
            if (entry == null) return list;
            var negotiated = NegociationUIManager.Instance != null ? NegociationUIManager.Instance.currentNegociatedItem : null;
            var back = entry.GetBackInvItems();
            foreach (var item in back)
            {
                if (item == null || !GeneralHelper.IsItemOwned(item)) continue;
                list.Add(Item(item, true, true, client != null && client.IsClientBuyingThisItem(item), item == negotiated));
            }
            foreach (var item in entry.GetAllNonOwnedItem())
            {
                if (item == null) continue;
                list.Add(Item(item, false, back.Contains(item), false, item == negotiated));
            }
            return list;
        }

        internal ItemInfo Item(GameItem item, bool owned, bool onBack, bool clientWants, bool negotiated)
        {
            var info = new ItemInfo
            {
                Uid = Uid(item),
                Id = item.identifier,
                Name = item.GetDisplayName(false),
                Price = item.GetNegociatedValue(),
                EstimatedValue = item.GetCurrentValue(false, true, true, false, 0L),
                Owned = owned,
                OnBack = onBack,
                ClientWants = clientWants,
                Negotiated = negotiated,
                ContrabandLevel = ContrabandHelper.GetContrabandLevelInInt(item),
                SeriousContraband = ContrabandHelper.IsSeriousContraband(item),
                AppraisalValue = AppraisalHelper.GetCorrectBaseValue(item),
            };
            int bought = item.GetTagReadonly("BOUGHT_PRICE_TAG").GetInt();
            if (bought > 0) info.BoughtAt = bought;
            if (StolenHelper.IsStolenItem(item)) info.StolenHeat = item.GetTagReadonly("STOLEN_VALUE_INT").GetInt();
            if (item.itemFeatures != null)
                foreach (var f in item.itemFeatures)
                    if (f != null) info.Features.Add(Feature(f));
            return info;
        }

        // Owned items on the shelves, in the window and directly inside a shelved smuggler bay: what the
        // player could stage for a buying customer. Members are listed under advise.stage.
        [MethodImpl(MethodImplOptions.NoInlining)]
        public IList<ShopItem> ShopItems(ClientInfo info)
        {
            var list = new List<ShopItem>();
            var entry = EmporiumEntry.Instance;
            var store = PlayerStore.Instance;
            var client = CurrentClient();
            if (entry == null || store == null || client == null || info == null) return list;
            // OnAddedToWeightedArea adds the retail markup for such clients; the rest of the deal price is set on staging.
            bool markup = store.IsCurrentClientAcceptMarkup();
            foreach (var item in entry.GetAllOwnedItems(false, false))
            {
                if (item == null || ContrabandHelper.IsSmugglerBay(item)) continue;
                string where = ShelfPlace(item, entry);
                if (where == null) continue;
                var shop = new ShopItem { Item = Item(item, true, false, client.IsClientBuyingThisItem(item), false), Where = where };
                shop.Item.Price = item.GetCurrentValue(markup, true, true, false, 0L);
                foreach (var tag in info.InterestTags)
                    if (tag != "ALL_TYPE" && item.IsTag(tag)) shop.Matches.Add(tag);
                list.Add(shop);
            }
            return list;
        }

        static string ShelfPlace(GameItem item, EmporiumEntry entry)
        {
            var inv = item.parentInventory;
            if (inv == null) return null;
            if (ReferenceEquals(inv, entry.invElement)) return StageWhere.Stock;
            if (ReferenceEquals(inv, entry.showcaseElement)) return StageWhere.Showcase;
            var parent = inv.GetParentItem();
            if (parent != null && ContrabandHelper.IsSmugglerBay(parent) && ReferenceEquals(parent.parentInventory, entry.invElement)) return StageWhere.SmugglerBay;
            return null;
        }

        // Items carry a uniqueId from creation; a 0 gets a negative id local to this run rather than
        // calling GetUniqueID(), which would assign one in the save.
        public int Uid(GameItem item)
        {
            int id = item.uniqueId;
            if (id != 0) return id;
            if (!localIds.TryGetValue(item, out id))
            {
                id = -(localIds.Count + 1);
                localIds[item] = id;
            }
            return id;
        }

        static FeatureInfo Feature(ItemFeature f)
        {
            bool exposed = f.IsExposed();
            var info = new FeatureInfo
            {
                Disabled = f.isDisabled,
                Visible = f.IsInitiallyShown() && !f.isDisabled && (!f.GetPublicHidden() || exposed),
                Group = Group(f.GetValueStage()),
                CurrentMod = f.GetCurrentValueMod(),
                ActualMod = f.GetActualValueModifier(),
                Real = f.GetBaseRealDisplay(),
            };
            try
            {
                info.Shown = exposed ? f.GetExposedString() : f.GetPublicDisplay();
            }
            catch (NullReferenceException)
            {
                // GetExposedString formats realCondition, which some non-condition features lack.
                info.Shown = f.GetPublicDisplay();
            }
            return info;
        }

        static FeatureGroup Group(ItemFeature.ValueStage stage) =>
            stage == ItemFeature.ValueStage.Market ? FeatureGroup.Market
            : stage == ItemFeature.ValueStage.Final ? FeatureGroup.Bargain
            : FeatureGroup.Condition;

        [MethodImpl(MethodImplOptions.NoInlining)]
        public BargainFlags Bargain()
        {
            var b = BargainUIManager.Instance;
            if (b == null) return new BargainFlags();
            return new BargainFlags
            {
                DiscountOffered = b.DiscountOffered,
                MarkupOffered = b.MarkupOffered,
                BuyingMarkupOffered = b.BuyingMarkupOffered,
                BuyingDiscountOffered = b.BuyingDiscountOffered,
                BlackmailOffered = b.BlackmailOffered,
                ThreatenOffered = b.ThreatenOffered,
            };
        }

        [MethodImpl(MethodImplOptions.NoInlining)]
        public UiState Ui()
        {
            var store = PlayerStore.Instance;
            var dialog = DialogUIManager.Instance;
            var mono = StoreClientMono.Instance;
            var storeUi = StoreUIManager.Instance;
            var map = MapUIManager.Instance;
            return new UiState
            {
                ClientPresent = store != null && store.currentClientInstance != null,
                ClientArrived = store != null && store.isClientArrived,
                DialoguePlaying = dialog != null && dialog.GetCurrentDialog() != null,
                ClientMoving = mono != null && mono.IsMoving(),
                AboutToStartMainDialog = storeUi != null && storeUi.isAboutToStartMainDialog,
                TradeLocked = NegociationUIManager.IsTradeLocked(),
                BargainIntroduced = store != null && store.isBargainIntroduced,
                AugIntroduced = store != null && store.isAugIntroduced,
                Outside = map != null && map.isOutside,
                AwfulNegotiator = StartingPerk.IsAwfulNegotiator(),
                LockNextBell = store != null ? store.lockNextBellFlag : 0,
            };
        }

        [MethodImpl(MethodImplOptions.NoInlining)]
        public IList<MarketEvent> MarketEvents()
        {
            var list = new List<MarketEvent>();
            var station = StoreStation.Instance;
            if (station == null || station.storeEventManager == null) return list;
            foreach (var ev in station.storeEventManager.GetActiveEvents())
            {
                if (ev == null || ev.negociationDatas == null) continue;
                foreach (var d in ev.negociationDatas)
                    if (d != null) list.Add(new MarketEvent { ItemType = d.itemType, ValuePct = d.value });
            }
            return list;
        }

        [MethodImpl(MethodImplOptions.NoInlining)]
        public Odds Odds(BargainKind kind, int percent)
        {
            var b = BargainUIManager.Instance;
            if (b == null) return new Odds();
            var type = (BargainType)(int)kind;
            return new Odds { Success = b.GetBargainSuccessChance(type, percent), Leave = b.GetBargainLeaveChance(type, percent) };
        }

        [MethodImpl(MethodImplOptions.NoInlining)]
        public int? DaysToInspection()
        {
            var store = PlayerStore.Instance;
            var manager = store != null ? store.storeClientManager : null;
            return manager != null ? manager.dayUntilInspection : (int?)null;
        }

        public void ForgetLocalIds() => localIds.Clear();
    }
}
