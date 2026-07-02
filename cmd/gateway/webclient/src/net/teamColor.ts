// Battle team colours by faction id, matching the legacy client's getTeamColor().
export function teamColor(factionId: number): string {
  switch (factionId) {
    case 1:
      return '#00f2fe'; // cyan
    case 2:
      return '#ffaa00'; // gold
    case 3:
      return '#ff007f'; // magenta
    case 4:
      return '#10b981'; // green
    case 5:
      return '#eab308'; // yellow
    default:
      return '#ffffff';
  }
}
