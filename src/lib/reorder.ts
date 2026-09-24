export function moveById<T extends { id: string }>(
  list: T[],
  dragId: string,
  targetId: string,
  after: boolean,
): T[] {
  if (dragId === targetId) return list;
  const from = list.findIndex((item) => item.id === dragId);
  if (from === -1) return list;
  const next = [...list];
  const [moved] = next.splice(from, 1);
  const target = next.findIndex((item) => item.id === targetId);
  if (target === -1) return list;
  next.splice(after ? target + 1 : target, 0, moved);
  return next;
}
