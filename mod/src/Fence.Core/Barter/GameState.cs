using System.Collections.Generic;

namespace Fence.Barter
{
    // What the client wants, in the game's StoreClient.ClientIntent terms. Other covers every intent
    // the first integration defers (barter, appraisal, story dialogue, ...).
    public enum ClientIntent
    {
        Other,
        Buy,
        Sell,
        SellAndBuy,
        Dialogue,
    }

    // Same order as the game's BargainType enum.
    public enum BargainKind
    {
        Discount,
        Markup,
        BuyingMarkup,
        BuyingDiscount,
        Blackmail,
        Threaten,
    }

    // ItemFeature.ValueStage: Innate, Market, Final.
    public enum FeatureGroup
    {
        Condition,
        Market,
        Bargain,
    }

    // Plain copies of game state, filled by the mod (Fence.Mod.GameBarter) or by a test fake. Null
    // means "could not be read" and is omitted from the context, never guessed.
    public sealed class StoreInfo
    {
        public int? Cash;
        public int? Day;
        public int? RetailMarkup;
        public int? RentDue;
        public int? RentDaysLeft;
    }

    public sealed class FactionRep
    {
        public string FactionId;
        public double Value;
        public string Status;
    }

    public sealed class ClientInfo
    {
        // Reference identity of the game's StoreClient; never serialized.
        public object Key;
        public string Name;
        public string Identifier;
        public string FactionId;
        public ClientIntent Intent;
        public string RawIntent;
        public bool UsesBudget;
        public int Budget;
        public List<string> InterestTags = new List<string>();
        public List<string> RestrictionTags = new List<string>();
        public bool NoContraband;
        public bool RequiresVariety;
        public bool IsSecurity;
        public bool IsWanted;
        public bool IsStealthWanted;
        public bool IsAug;
        public string AttractedItemId;
        public string AttractedType;
        public bool OffersWholesale;
        // Game convention: negative, e.g. -15.
        public int WholesaleDiscount;
        public int? WholesalePrice;
        public bool Dismissable;
        public bool BargainAvailable;
        public bool MainDialogueStarted;
        public bool BusinessComplete;
        public bool RegularBuyDisabled;
        public bool MultiBuyDisabled;
    }

    public sealed class FeatureInfo
    {
        // Shown on the tablet: IsInitiallyShown && !isDisabled && (!publicHidden || exposed).
        public bool Visible;
        public bool Disabled;
        // The tablet's line, raw (rich text allowed): exposed ? GetExposedString : GetPublicDisplay.
        public string Shown;
        // The true condition (GetBaseRealDisplay); hidden until exposed.
        public string Real;
        public FeatureGroup Group;
        public int CurrentMod;
        public int ActualMod;
    }

    public sealed class ItemInfo
    {
        public int Uid;
        public string Id;
        public string Name;
        public long Price;
        public long? EstimatedValue;
        // BOUGHT_PRICE_TAG; 0 or null = unknown.
        public int? BoughtAt;
        public bool Owned;
        public bool OnBack;
        // StoreClient.IsClientBuyingThisItem, for owned items on the back counter.
        public bool ClientWants;
        // True for NegociationUIManager.currentNegociatedItem (drives the SELLNBUY tablet mode).
        public bool Negotiated;
        public int? StolenHeat;
        public int ContrabandLevel;
        public bool SeriousContraband;
        // AppraisalHelper.GetCorrectBaseValue; 0 = not an appraisal item.
        public int AppraisalValue;
        public List<FeatureInfo> Features = new List<FeatureInfo>();
    }

    public sealed class BargainFlags
    {
        public bool DiscountOffered;
        public bool MarkupOffered;
        public bool BuyingMarkupOffered;
        public bool BuyingDiscountOffered;
        public bool BlackmailOffered;
        public bool ThreatenOffered;

        public bool SellSideUsed => DiscountOffered || MarkupOffered;
        public bool BuySideUsed => BuyingMarkupOffered || BuyingDiscountOffered;
        public bool Coerced => BlackmailOffered || ThreatenOffered;

        public bool Offered(BargainKind kind)
        {
            switch (kind)
            {
                case BargainKind.Discount: return DiscountOffered;
                case BargainKind.Markup: return MarkupOffered;
                case BargainKind.BuyingMarkup: return BuyingMarkupOffered;
                case BargainKind.BuyingDiscount: return BuyingDiscountOffered;
                case BargainKind.Blackmail: return BlackmailOffered;
                default: return ThreatenOffered;
            }
        }
    }

    public sealed class UiState
    {
        public bool ClientPresent;
        public bool ClientArrived;
        public bool DialoguePlaying;
        public bool ClientMoving;
        public bool AboutToStartMainDialog;
        public bool TradeLocked;
        public bool BargainIntroduced;
        public bool AugIntroduced;
        public bool Outside;
        public bool AwfulNegotiator;
        public int LockNextBell;
    }

    public sealed class MarketEvent
    {
        public string ItemType;
        public int ValuePct;
    }

    public sealed class Odds
    {
        public int Success;
        public int Leave;
    }

    // Every game read the barter advisor needs. The mod implements it over the live game; tests use
    // fakes. Implementations may throw; callers treat that as "not readable now".
    public interface IBarterGame
    {
        StoreInfo Store();
        IList<FactionRep> Reputations();
        ClientInfo Client();
        // Both counters: the client's items (front and back) and the shop's items on the back counter.
        IList<ItemInfo> CounterItems();
        BargainFlags Bargain();
        UiState Ui();
        IList<MarketEvent> MarketEvents();
        // BargainUIManager.GetBargainSuccessChance / GetBargainLeaveChance: what the button tooltip shows.
        Odds Odds(BargainKind kind, int percent);
        int? DaysToInspection();
    }
}
