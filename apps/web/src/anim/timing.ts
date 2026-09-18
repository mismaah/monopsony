// Shared durations (ms) so the animation queue and the 3D components agree.
export const T = {
  diceRoll: 900,
  tokenStep: 170,
  tokenTeleport: 600,
  cardShow: 2200,
  highlight: 500,
  buildingPop: 250,
  /** How long a floating +$/-$ pop stays on the players rail before sliding out. */
  cashPop: 2000,
} as const;
