namespace Fence.Gym
{
    // The one ModSide helper the shared executor uses (Appraisal.cs); the gym's ModSide is its protocol driver
    // and stays gym-only.
    internal static class ModSide
    {
        public static int CurrentDay() => StoreStation.Instance != null ? StoreStation.Instance.dayCounter : 0;
    }
}
