import { AppInfoService } from '../../../bindings/github.com/chenyang-zz/cove/internal/services'

export type AppInfo = {
  name: string
  version: string
  platform: string
  arch: string
}

export async function getAppInfo(): Promise<AppInfo> {
  return AppInfoService.GetAppInfo()
}
