/**
 * Command palette open state. A separate store because both the global
 * keyboard handler and the palette component itself need to read/write it.
 */

class PaletteStore {
  open = $state(false);

  toggle(): void {
    this.open = !this.open;
  }

  show(): void {
    this.open = true;
  }

  hide(): void {
    this.open = false;
  }
}

export const palette = new PaletteStore();
