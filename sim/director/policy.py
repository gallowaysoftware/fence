"""The director: a deterministic day policy for the gym's step mode (docs/research/DIRECTOR.md).

decide(snapshot, memory, params) -> (directives, reasons). Pure in its inputs: the same snapshot
history and parameters always give the same directives, so a seed replays bit-exact.
The plan is the vintner + mortgage route of the first Claude win (docs/research/CLAUDE-PLAY.md,
round 2 player 5): wine engine and favor upgrades, Georgio's 4x loan, build to the loan shark's
rent-day offer, take the mortgage only with a cushion, then twelve payments.
"""
from dataclasses import dataclass, field, fields, asdict
import json

FACTIONS = ('lower_levels', 'upper_levels', 'security', 'black_market', 'revolution')
KIT = {'hydroponic', 'wine_bottle', 'wine_yeast', 'wine_superyeast', 'wine_yeast_infinite', 'bloomberry_seed',
       'nutrient_tablet', 'nutrient_tablet_container', 'energy_credit', 'water_filter', 'water_pitcher',
       'water_tablet_container', 'water_test_container', 'water_ration', 'water_ration_small', 'water_jug', 'wine_rack'}
YEASTS = ('wine_yeast', 'wine_superyeast', 'wine_yeast_infinite')
BOTTLED_WATER = ('large_bottled_water', 'bottled_water', 'small_bottled_water')
GEORGIO_LOAN_DAY = 43  # StorylineSchedule.StartDay("georgio") (6) + 37: he borrows cash / 4, repays 4x after day +25


@dataclass
class Params:
    # Opening and engine
    engine: str = 'wine'                 # 'wine' | 'none'; hard overrides via hard_engine
    hard_engine: str = 'none'
    bays: int = 3
    services: list = field(default_factory=lambda: ['INSURANCE', 'JANITORIAL', 'BUY_REFERAL', 'SELL_REFERAL'])
    services_mode: str = ''              # 'ev' hands services to the executor's EV rule instead of a list
    bargain: str = 'ev_positive'
    ll_floor: float = -10.0              # Lower Levels rep below this: bargain ll_bargain
    ll_bargain: str = 'never'            # 'discount' still demands discounts from sellers (walk-outs, -LL)
    augs_normal: str = 'shoot'
    augs_hard: str = 'shoot'
    liquidate_min_pct: int = 60
    # Upgrades: favor in strict order; cash only for the listed ones
    upgrades: list = field(default_factory=lambda: [
        'WINEMAKER', 'WATER_TRADER', 'SHOWCASE_I', 'BUY_REFERAL', 'SELL_REFERAL', 'RENOVATION1', 'SHOWCASE_II',
        'RENOVATION2', 'RENOVATION3', 'MARKETPLACE_REQUESTS', 'BREWER', 'SEC_GRACE', 'MINER', 'MINER_SHAREHOLDER',
        'BREWER_SHAREHOLDER'])
    cash_ok: list = field(default_factory=lambda: ['WINEMAKER'])
    cash_ok_reserve: int = -150          # cash an engine upgrade may leave below the rent reserve
    # Permit
    permit_cushion: int = 1500           # buy only when cash - fee covers what is owed this week + this
    permit_mort_payments: float = 2.0    # under the mortgage: cash - fee >= this many payments
    # The executor buys the permit on 'bribe' once bribing fails and cash covers fee + rent (the rent,
    # not the mortgage payment): 'refuse' on those days unless the director wants the buy.
    permit_refuse_when_failing: bool = False
    # Georgio (day 43): no buying the two days before, sell down so cash/4 is as large as possible
    georgio_prep: bool = True
    georgio_prep_days: int = 2
    georgio_sell_pct: int = 75
    georgio_sell_frac: float = 0.7
    # Building toward the loan shark's rent-day offer (cash >= 2x down payment that morning)
    build_from: int = 35
    build_days: int = 4
    liq_days: int = 3
    build_liq: float = 0.7
    build_margin: int = 150
    accum_from: int = 38                 # cap trading stock from here to accumulate cash pre-mortgage
    accum_cap: int = 1800
    # Mortgage take rule: cash after the down payment >= take_payments x weekly + take_float,
    # counting Georgio's pending 4x payback
    take_payments: float = 1.5
    take_float: int = 1000
    take_count_georgio: bool = True
    take_count_permit: bool = True       # no permit yet: the executor will buy it (1500) once bribing fails
    mortgage_last_day: int = 999         # never take it after this day
    # Payments and rent
    pay_margin: int = 100
    mort_floor: int = 200                # explicit floor on ordinary mortgage days (unset = one payment: buys nothing)
    sell_days_rent: int = 3
    sell_days_mort: int = 4
    floor_days: int = 2
    # Supplier day (bottles, yeast, seeds come late in the day): keep the cash
    supplier_days: list = field(default_factory=lambda: [5])
    supplier_slack: int = 150
    # Cellar: hold the best wines only while liquid
    liquid_extra: int = 600
    max_hold: int = 40
    # Grid release: berries piling up while no bottle can start
    berry_keep: int = 6
    berry_slack: int = 6
    water_keep: int = 4
    # Stall: no wine started for this many days -> engine off except on supplier days
    stall_days: int = 10
    # Evidence 100 with crime >= 300 is an arrest: EVIDENCE_REMOVAL (favor) jumps the queue from here
    evidence_removal_at: int = 50

    # 'full' plays the plan above; 'thin' defers to the executor (d9c4d40 and later already carries the
    # Georgio peak, mortgage float, repo reserve and evidence rules) and adds only the permit rule below.
    mode: str = 'full'
    thin_permit_cushion: int = 300       # thin: buy the permit on a visit day once cash - fee >= rent + this
    thin_extra: dict = field(default_factory=dict)  # thin: extra executor directives passed through

    @classmethod
    def load(cls, overrides):
        p = cls()
        names = {f.name for f in fields(cls)}
        for k, v in (overrides or {}).items():
            if k not in names:
                raise KeyError('unknown director param ' + k)
            setattr(p, k, v)
        return p

    def to_json(self):
        return asdict(self)


class Memory:
    """What the director remembers across days of one game (all derived from earlier snapshots/results)."""

    def __init__(self):
        self.georgio_lent = 0
        self.georgio_repaid = False
        self.last_started = None
        self.last_start_day = 0
        self.mortgage_day = None

    def observe(self, snap):
        y = snap.get('yesterday') or {}
        cf = y.get('cash_flow') or {}
        if cf.get('georgio_loan'):
            self.georgio_lent = cf['georgio_loan']
        if cf.get('georgio_payback'):
            self.georgio_repaid = True
        pw = wine_prod(y)
        st = pw.get('started_total')
        if st is not None:
            if self.last_started is None or st > self.last_started:
                self.last_start_day = snap['day']
            self.last_started = st
        m = snap['risks'].get('mortgage') or {}
        if m.get('paying') and self.mortgage_day is None:
            self.mortgage_day = snap['day']

    @property
    def georgio_pending(self):
        return 0 if self.georgio_repaid else 4 * self.georgio_lent


def wine_prod(y):
    p = (y or {}).get('production')
    return (p.get('wine') or {}) if isinstance(p, dict) else {}


def top_inventory(s):
    return [i for i in s['inventory'] if not str(i.get('where', '')).startswith('inside') and i.get('where') not in ('docs', 'trash')]


def est(i):
    return max(0, i.get('estimated_value') or 0)


def finished_wines(top):
    return [i for i in top if i['id'] == 'wine_bottle' and est(i) >= 60]


def owed(snap):
    """(amount due next, days left, paying the mortgage)."""
    r = snap['risks']
    m = r.get('mortgage') or {}
    if m.get('paying'):
        return m['weekly'], r['rent']['days_left'], True
    return r['rent']['due'], r['rent']['days_left'], False


def decide_thin(snap, mem, p):
    s, r = snap['snapshot'], snap['risks']
    st = s['store']
    day = snap['day']
    d = {'day': day}
    d.update(p.thin_extra)
    why = []
    # The permit official's arrests end hard runs once bribing fails; Georgio's payback (~day 56-70)
    # is the first time the fee is affordable, so buy on the next visit rather than wait for a surplus.
    if not st['business_permit']:
        fee, due = r['permit']['fee'], r['rent']['due']
        if r['permit'].get('next_visit_day') == day and st['cash'] - fee >= due + p.thin_permit_cushion:
            d['permit'] = 'buy'
            if r['rent']['days_left'] > 2:
                d['cash_floor'] = 0  # the executor's buy check adds the floor to fee + rent
            why.append(f"permit buy (cash {st['cash']}, fee {fee})")
        else:
            d['permit'] = 'bribe'
    return d, why


def decide(snap, mem, p):
    if p.mode == 'thin':
        return decide_thin(snap, mem, p)
    s = snap['snapshot']
    r = snap['risks']
    st = s['store']
    day = snap['day']
    hard = snap.get('difficulty') in ('hard', 'challenge')
    cash = st['cash']
    favor = st.get('wild_favor') or 0
    due, left, mort = owed(snap)
    m = r.get('mortgage') or {}
    why = []

    d = {'day': day, 'bargain': p.bargain, 'augs': p.augs_hard if hard else p.augs_normal, 'scavenge': True,
         'bays': p.bays, 'growth': [], 'sell': {'liquidate_min_pct': p.liquidate_min_pct}, 'buy': {}}
    engine = p.hard_engine if hard else p.engine
    d['engine'] = engine
    buy, sell = d['buy'], d['sell']

    svc_cost = sum(x['cost'] for x in s['services'] if x['using'])
    reserve = due + svc_cost * (left + 1)
    top = top_inventory(s)
    stock = sum(est(i) for i in top)
    engine_stock = sum(est(i) for i in top if i['id'] in KIT or i['id'] == 'wine_berry')
    trade_stock = stock - engine_stock

    # Services: an explicit list, or the executor's EV rule.
    if p.services_mode:
        d['services_mode'] = p.services_mode
    elif p.services:
        unlocked = {x['id'].replace('_SERVICE', '') for x in s['services'] if x['unlocked']}
        sv = [x for x in p.services if x.replace('_SERVICE', '') in unlocked]
        if sv:
            d['services'] = sv

    # Lower Levels customers walk out and leave bad reviews below -20: stop demanding discounts first.
    ll = s['reputation']['lower_levels']['value']
    if ll < p.ll_floor:
        d['bargain'] = p.ll_bargain
        why.append(f'LL {ll:.0f}: bargain {p.ll_bargain}')

    # Permit: set every day (unset lets the executor buy it blind to the mortgage). Buy only on the
    # official's visit day with a cushion; the floor keeps the fee from being spent before he comes.
    permit_floor = 0
    if not st['business_permit']:
        fee = r['permit']['fee']
        need = reserve + p.permit_cushion
        if mort:
            need = max(need, int(p.permit_mort_payments * due) + svc_cost * 7)
        visit = r['permit'].get('next_visit_day') == day
        if visit and cash - fee >= need:
            d['permit'] = 'buy'
            # The executor buys only while cash >= fee + rent + cash_floor (the rent, even under the
            # mortgage), so the floor is the cushion alone; a floor including the fee blocks the buy.
            permit_floor = max(0, need - r['rent']['due'])
            why.append(f'permit buy (cash {cash} - fee {fee} >= {need})')
        elif p.permit_refuse_when_failing and r['permit'].get('bribe_failing'):
            d['permit'] = 'refuse'
        else:
            d['permit'] = 'bribe'

    # Mortgage: take it only with a cushion after the down payment (Georgio's pending payback counts).
    building = False
    if not mort:
        down, weekly = m.get('down_payment', 2000), m.get('weekly', 1000)
        pending = mem.georgio_pending if p.take_count_georgio else 0
        need_cash = down + int(p.take_payments * weekly) + p.take_float - pending
        if p.take_count_permit and not st['business_permit']:
            need_cash += r['permit']['fee']
        target = max(m.get('offered_at_cash', 2 * down), need_cash) + p.build_margin
        take = cash >= max(need_cash, down + weekly) and day <= p.mortgage_last_day
        d['mortgage'] = 'take' if take else 'later'
        if left == 0 and m.get('offerable_now'):
            why.append(f"mortgage {'take' if take else 'later'} (cash {cash}, need {need_cash}, georgio {pending})")
        # Build cash for the rent-day offer: the morning check reads the cash left the night before.
        if day >= p.build_from and day <= p.mortgage_last_day and 1 <= left <= p.build_days \
                and cash + p.build_liq * trade_stock >= target:
            building = True
            d['cash_floor'] = min(cash, target)
            buy['max_spend'] = max(0, cash - target)
            if left <= p.liq_days:
                sell['sell_down_to_cash'] = target
            why.append(f'build to {target} for the rent-day offer')

    # Georgio borrows a quarter of the cash on day 43 and repays four times: have the cash in hand.
    georgio = p.georgio_prep and not mort and GEORGIO_LOAN_DAY - p.georgio_prep_days <= day <= GEORGIO_LOAN_DAY
    if georgio:
        buy['max_spend'] = 0
        d['cash_floor'] = cash
        if day < GEORGIO_LOAN_DAY:
            sell['sell_down_to_cash'] = cash + int(p.georgio_sell_frac * trade_stock)
            sell['liquidate_min_pct'] = p.georgio_sell_pct
        why.append('georgio: cash in hand for day 43')

    supplier = (day % 7) in p.supplier_days and not building and not georgio
    if supplier:
        buy['stock_cap'] = max(0, trade_stock)
        d['cash_floor'] = max(0, reserve - p.supplier_slack) if left <= p.floor_days else 0
        why.append(f'supplier day: trading capped at {trade_stock}')

    accum = not mort and day >= p.accum_from and day <= p.mortgage_last_day
    if accum and not supplier and not georgio:
        buy['stock_cap'] = p.accum_cap
        why.append(f'accumulating: trading stock capped at {p.accum_cap}')

    margin = p.pay_margin if mort else 0
    if not building and not supplier and not georgio:
        if left <= p.floor_days:
            d['cash_floor'] = min(cash, reserve + margin)
            why.append(f'due {due} in {left}d: floor {d["cash_floor"]}')
        if left <= (p.sell_days_mort if mort else p.sell_days_rent) and cash < reserve + margin:
            sell['sell_down_to_cash'] = reserve + margin
            why.append(f'sell down to {reserve + margin}')
    if mort and 'cash_floor' not in d:
        d['cash_floor'] = min(cash, p.mort_floor)
    if permit_floor:
        d['cash_floor'] = min(d.get('cash_floor', permit_floor), permit_floor)

    # Cellar: hold the best wines only while the cash covers what is owed and a buying float.
    wines = finished_wines(top)
    liquid = cash >= reserve + p.liquid_extra and not building and not accum and not georgio
    hold = []
    if wines and liquid:
        keep = sorted(wines, key=lambda i: (-est(i), i['uid']))[:p.max_hold]
        hold = [i['uid'] for i in keep]
        why.append(f'hold {len(keep)} wines')

    # Engine: stalled (nothing started for stall_days) -> off except on supplier days, so its
    # inputs are sold; berries piling up while no bottle can start -> off for the day.
    if engine == 'wine':
        y = snap.get('yesterday') or {}
        pw = wine_prod(y)
        berries = [i for i in top if i['id'] == 'wine_berry']
        empty = sum(1 for i in top if i['id'] == 'wine_bottle' and est(i) < 20)
        water_ml = sum(750 if i['id'] == 'water_ration' else 375 for i in top if i['id'] in ('water_ration', 'water_ration_small')) \
            + (pw.get('clean_water_ml') or 0)
        can_start = min(empty, water_ml // 750) > 0 and any(i['id'] in YEASTS for i in top)
        # Wine off only when it cannot restart: nothing started lately, nothing fermenting, no seed growing.
        seeds = any(i['id'] == 'bloomberry_seed' for i in top) or (pw.get('bay_seeded') or 0) > 0
        stalled = day >= 20 and day - mem.last_start_day >= p.stall_days and not seeds and not pw.get('fermenting')
        keep_b = max(p.berry_keep, empty + 2)
        if stalled and not supplier:
            d['engine'] = 'none'
            why.append(f'engine stalled since day {mem.last_start_day}: off')
        elif len(berries) > keep_b + p.berry_slack and not can_start and not supplier:
            water = [i for i in top if i['id'] in BOTTLED_WATER]
            kit = [i['uid'] for i in top if i['id'] in KIT and not (i['id'] == 'wine_bottle' and est(i) >= 20)]
            hold += kit + [i['uid'] for i in water[:p.water_keep]] + [i['uid'] for i in berries[:keep_b]]
            d['engine'] = 'none'
            why.append(f'grid release: engine off, sell {len(berries) - keep_b} berries')
    if hold:
        sell['hold_uids'] = sorted(set(hold))

    # Upgrades: favor in strict order (it waits for the next one rather than leaking to cheaper ones).
    owned = {u['id'] for u in s['upgrades']['owned'] if u.get('state') == 2}
    cost = {u['id']: u['cost'] for u in (r.get('wild') or {}).get('available', [])}
    picks, f, c = [], favor, cash
    order = list(p.upgrades)
    if r['record']['evidence'] >= p.evidence_removal_at:
        order.insert(0, 'EVIDENCE_REMOVAL')
        why.append(f"evidence {r['record']['evidence']}: EVIDENCE_REMOVAL first")
    for u in order:
        if u in owned or u not in cost:
            continue
        if f >= cost[u]:
            picks.append(u)
            f -= cost[u]
        elif u in p.cash_ok and not building and not georgio and f + c - cost[u] >= reserve + p.cash_ok_reserve:
            picks.append(u)
            c -= cost[u] - f
            f = 0
        else:
            break
    d['upgrades_buy'] = picks
    if picks:
        why.append('upgrades ' + ','.join(picks))

    if not buy:
        del d['buy']
    return d, why


def dumps(d):
    return json.dumps(d, sort_keys=True)
