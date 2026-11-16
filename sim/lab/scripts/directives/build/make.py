#!/usr/bin/env python3
"""Writes this directory's directives: a route file plus overrides (docs/research/BUILD-ORDER.md). Rerun after a route changes."""
import json
import os

HERE = os.path.dirname(os.path.abspath(__file__))
D = os.path.join(HERE, "..")


def load(rel):
    with open(os.path.join(D, rel)) as f:
        return json.load(f)


def write(name, base, **over):
    d = dict(load(base))
    for k, v in over.items():
        if v is None:
            d.pop(k, None)
        else:
            d[k] = v
    with open(os.path.join(HERE, name + ".json"), "w") as f:
        json.dump(d, f, indent=1)
        f.write("\n")


LAB = {"products": ["caffeine_pill"]}
COFFEE = {"coffee_bean": 2}
NB = {"appraisal": None}

# (b) the cellar
write("vintner_cw_build", "cellar/vintner_cw.json", build={})
write("vintner_cw_must", "cellar/vintner_cw.json", build={"must": ["water"]})
# (a) the lab, beans from visitors and from crop bays
write("lab_build", "lab/caffeine_lab.json", build={})
write("lab_must", "lab/caffeine_lab.json", build={"must": ["lab"]})
write("lab_crops", "lab/caffeine_lab.json", crops=COFFEE)
write("lab_crops_must", "lab/caffeine_lab.json", crops=COFFEE, build={"must": ["lab", "crop_bays"]})

# (a) the farmer start: its bays come with the start; lab and coffee crops on them, with and without the Board
write("farmer_lab", "routes/farmer.json", lab=LAB)
write("farmer_lab_must", "routes/farmer.json", lab=LAB, build={"must": ["lab"]})
write("farmer_lab_crops", "routes/farmer.json", lab=LAB, crops=COFFEE)
write("farmer_lab_crops_must", "routes/farmer.json", lab=LAB, crops=COFFEE, build={"must": ["lab", "crop_bays"]})
write("farmer_nb", "routes/farmer.json", **NB)
write("farmer_lab_crops_nb", "routes/farmer.json", lab=LAB, crops=COFFEE, **NB)
write("farmer_lab_crops_build_nb", "routes/farmer.json", lab=LAB, crops=COFFEE, build={}, **NB)
write("sprinter_nb", "routes/sprinter.json", **NB)
write("lab_crops_nb", "lab/caffeine_lab.json", crops=COFFEE, **NB)
write("lab_crops_build_nb", "lab/caffeine_lab.json", crops=COFFEE, build={}, **NB)
write("vintner_nb", "cellar/vintner.json", **NB)
write("vintner_cw_nb", "cellar/vintner_cw.json", **NB)
write("vintner_cw_build_nb", "cellar/vintner_cw.json", build={}, **NB)

# (c) the armourer's forge
write("armourer_forge_build", "arms/armourer_forge.json", build={})
write("armourer_forge_must", "arms/armourer_forge.json", build={"must": ["forge"]})
write("armourer_forge_must_ins", "arms/armourer_forge.json", build={"must": ["forge"], "insure_kit": 400})

# The horizon at the Board routes' measured median win day (98): the planner decides with the run it will really have.
write("lab_crops_build_h98", "lab/caffeine_lab.json", crops=COFFEE, build={"horizon": 98})
write("vintner_cw_build_h98", "cellar/vintner_cw.json", build={"horizon": 98})
write("farmer_lab_crops_build_h98", "routes/farmer.json", lab=LAB, crops=COFFEE, build={"horizon": 98})

# ---- after main's power (Power.cs), lab supply and breeding merges ----
POWER = {}
WEED = {"lines": ["caffeine_weed"]}
FW = {"fillerweed_seed": 2}
B = {}  # the planner, every default
# (a) caffeine weed + lab: apprentice and farmer, with and without the Board
write("weed_power", "breeding/caffeine_weed.json", power=POWER)
write("weed_power_build", "breeding/caffeine_weed.json", power=POWER, build=B)
write("weed_power_must", "breeding/caffeine_weed.json", power=POWER, build={"must": ["dna"]})
write("weed_power_nb", "breeding/caffeine_weed.json", power=POWER, **NB)
write("weed_power_build_nb", "breeding/caffeine_weed.json", power=POWER, build=B, **NB)
write("weed_power_must_nb", "breeding/caffeine_weed.json", power=POWER, build={"must": ["dna"]}, **NB)
write("farmer_weed", "routes/farmer.json", lab=LAB, crops=FW, breeding=WEED, power=POWER)
write("farmer_weed_build", "routes/farmer.json", lab=LAB, crops=FW, breeding=WEED, power=POWER, build=B)
write("farmer_weed_must", "routes/farmer.json", lab=LAB, crops=FW, breeding=WEED, power=POWER, build={"must": ["dna"]})
write("farmer_weed_nb", "routes/farmer.json", lab=LAB, crops=FW, breeding=WEED, power=POWER, **NB)
write("farmer_weed_build_nb", "routes/farmer.json", lab=LAB, crops=FW, breeding=WEED, power=POWER, build=B, **NB)
write("farmer_weed_must_nb", "routes/farmer.json", lab=LAB, crops=FW, breeding=WEED, power=POWER, build={"must": ["dna"]}, **NB)
# the lab's own supply loop (coffee crops, power)
write("loop_build", "lab/caffeine_loop.json", build=B)
write("loop_must", "lab/caffeine_loop.json", build={"must": ["lab", "crop_bays"]})
# (b) the cellar with Power.cs
write("vintner_cw_power", "cellar/vintner_cw.json", power=POWER)
write("vintner_cw_power_build", "cellar/vintner_cw.json", power=POWER, build=B)
write("vintner_cw_power_must", "cellar/vintner_cw.json", power=POWER, build={"must": ["water"]})

# (d) every engine stacked on each native route, without and with the planner
CW = {"tap": "gutter"}
FORGE = load("arms/armourer_forge.json")["arms"]
write("farmer_all", "routes/farmer.json", lab=LAB, crops=FW, breeding=WEED, power=POWER, clean_water=CW)
write("farmer_all_build", "routes/farmer.json", lab=LAB, crops=FW, breeding=WEED, power=POWER, clean_water=CW, build=B)
write("apprentice_all", "breeding/caffeine_weed.json", power=POWER, clean_water=CW)
write("apprentice_all_build", "breeding/caffeine_weed.json", power=POWER, clean_water=CW, build=B)
write("armourer_all", "arms/armourer_forge.json", power=POWER, clean_water=CW)
write("armourer_all_build", "arms/armourer_forge.json", power=POWER, clean_water=CW, build=B)
write("water_baron_all", "cellar/water_baron_cw.json", power=POWER)
write("water_baron_all_build", "cellar/water_baron_cw.json", power=POWER, build=B)
write("farmer_weed_build_h98", "routes/farmer.json", lab=LAB, crops=FW, breeding=WEED, power=POWER, build={"horizon": 98})

# Diagnostic: is the counter's lost buying the stock grid (the liquidation threshold at 36 free cells)?
write("sprinter_mfc0", "routes/sprinter.json", min_free_cells=0)
write("weed_power_build_mfc0", "breeding/caffeine_weed.json", power=POWER, build=B, min_free_cells=0)
write("weed_power_mfc0", "breeding/caffeine_weed.json", power=POWER, min_free_cells=0)

# (d) on the Gauntlet's native routes (main a34b00b): every engine stacked, without and with the planner
ARMS_FORGE = dict(load("routes/armourer.json")["arms"], **{k: v for k, v in FORGE.items()})
for route, extra in [
    ("appraiser", dict(lab=LAB, crops=FW, breeding=WEED, power=POWER, clean_water=CW)),
    ("farmer", dict(lab=LAB, crops=FW, breeding=WEED, power=POWER, clean_water=CW)),
    ("armourer", dict(arms=ARMS_FORGE, power=POWER, clean_water=CW)),
    ("water_baron", dict(power=POWER, clean_water={})),
]:
    write(f"g_{route}_all", f"routes/{route}.json", **extra)
    write(f"g_{route}_all_build", f"routes/{route}.json", build=B, **extra)
    write(f"g_{route}_build_h", f"routes/{route}.json", build={"horizon": 98}, **extra)

# The counter's money: the Board route's savings are the stock money for big wholesale lots (morning cash days 1-90
# averages ~7 000 on `sprinter`, ~2 700 with the weed line). Hold 30 days of the counter's purchases instead of 4.
write("weed_power_build_t30", "breeding/caffeine_weed.json", power=POWER, build={"trade_days": 30})
write("farmer_weed_build_t30", "routes/farmer.json", lab=LAB, crops=FW, breeding=WEED, power=POWER, build={"trade_days": 30})
write("farmer_weed_build_h98_t30", "routes/farmer.json", lab=LAB, crops=FW, breeding=WEED, power=POWER, build={"horizon": 98, "trade_days": 30})
