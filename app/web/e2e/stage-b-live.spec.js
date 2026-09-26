import { test } from '@playwright/test'

test('b31 live full journey', async () => {
  test.skip(!process.env.ZHIGU_E2E_API, '需要本机 Go+Python+Postgres 与 D-02/D-05；安装浏览器不能代替 live 全链路')
})
