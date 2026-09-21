export function fillSide(fill, orders = []) {
  const id = fill?.order_id || fill?.orderId
  const order = (orders || []).find((row) => row && row.id === id)
  const side = String(order?.side || '').toLowerCase()
  if (side === 'buy' || side === 'sell') return side
  const cash = Number(fill?.cash_delta ?? fill?.CashDelta)
  if (Number.isFinite(cash) && cash !== 0) return cash < 0 ? 'buy' : 'sell'
  const qty = Number(fill?.qty ?? fill?.Qty)
  if (Number.isFinite(qty) && qty < 0) return 'sell'
  return 'buy'
}
