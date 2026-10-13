// New instance's "Share with other instances": the design's fixed list, each label
// naming the shared target it joins ("keys" is the shared keybinds switch).
export const NEW_INSTANCE_SHARES = [
  { label: 'Keybinds', id: 'keys' },
  { label: 'Game options', id: 'options' },
  { label: 'Servers', id: 'servers' },
  { label: 'Worlds', id: 'saves' },
  { label: 'Resource packs', id: 'resourcepacks' },
  { label: 'Shaders', id: 'shaderpacks' },
  { label: 'Screenshots', id: 'screenshots' },
  { label: 'Hotbars', id: 'hotbar' },
] as const;
