const API_BASE_URL = '/api'

export interface GetConfigResponse {
  app_id: string
  token: string
  uid: string
  channel_name: string
  agent_uid: string
}

export async function getConfig(options?: { uid?: string | number }): Promise<GetConfigResponse> {
  const params = new URLSearchParams()
  if (options?.uid !== undefined && options.uid !== '') params.set('uid', String(options.uid))
  const query = params.toString()
  const response = await fetch(`${API_BASE_URL}/get_config${query ? `?${query}` : ''}`)
  if (!response.ok) throw new Error(`HTTP ${response.status}`)
  const result = await response.json()
  if (result.code !== 0 || !result.data) throw new Error(result.msg || 'Failed to get configuration')
  return result.data as GetConfigResponse
}
