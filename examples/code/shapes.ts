type Shape =
  | { kind: 'circle'; radius: number }
  | { kind: 'square'; side: number };

function area(s: Shape): number {
  switch (s.kind) {
    case 'circle':
      return Math.PI * s.radius ** 2;
    case 'square':
      return s.side ** 2;
  }
}

const shapes: Shape[] = [
  { kind: 'circle', radius: 1 },
  { kind: 'square', side: 2 },
];
for (const s of shapes) {
  console.log(s.kind, area(s).toFixed(2));
}
