// SPDX-License-Identifier: BSD-3-Clause

import React, { useCallback, useState } from "react"
import { useAPI } from "src/api"
import { NodeData } from "src/types"
import Button from "src/ui/button"
import Input from "src/ui/input"
import Toggle from "src/ui/toggle"

type InitialServerConfig = {
  serverIP: string
  serverPort: string
  useHTTPS: boolean
}

function initialServerConfig(controlURL: string): InitialServerConfig {
  if (!controlURL) {
    return { serverIP: "", serverPort: "80", useHTTPS: false }
  }
  try {
    const u = new URL(controlURL)
    const useHTTPS = u.protocol === "https:"
    return {
      serverIP: u.hostname,
      serverPort: u.port || (useHTTPS ? "443" : "80"),
      useHTTPS,
    }
  } catch {
    return { serverIP: "", serverPort: "80", useHTTPS: false }
  }
}

function bracketIPv6Host(host: string) {
  return host.includes(":") && !host.startsWith("[") ? `[${host}]` : host
}

/**
 * ServerConfigView replaces the original LoginView.
 * It lets users configure the control server URL without invoking
 * `scaletail up --login-server=...` from a shell.
 */
export default function ServerConfigView({ data }: { data: NodeData }) {
  const api = useAPI()
  const initial = initialServerConfig(data.ControlURL)

  const [serverIP, setServerIP] = useState(initial.serverIP)
  const [serverPort, setServerPort] = useState(initial.serverPort)
  const [useHTTPS, setUseHTTPS] = useState(initial.useHTTPS)
  const [authKey, setAuthKey] = useState("")
  const [connecting, setConnecting] = useState(false)
  const [error, setError] = useState("")

  const needsAuth = data.Status === "NeedsLogin"

  const buildControlURL = useCallback(() => {
    const scheme = useHTTPS ? "https" : "http"
    const port = serverPort.trim()
    const host = bracketIPv6Host(serverIP.trim())
    return `${scheme}://${host}:${port}`
  }, [serverIP, serverPort, useHTTPS])

  const handleConnect = useCallback(async () => {
    const host = serverIP.trim()
    const port = Number(serverPort)
    if (!host) {
      setError("请输入服务器地址")
      return
    }
    if (!Number.isInteger(port) || port < 1 || port > 65535) {
      setError("请输入 1-65535 之间的端口")
      return
    }

    setError("")
    setConnecting(true)

    try {
      await api({
        action: "up",
        data: {
          ControlURL: buildControlURL(),
          AuthKey: authKey.trim() || undefined,
          Reauthenticate: !authKey.trim(),
        },
      })
    } catch (err) {
      setError(String(err))
    } finally {
      setConnecting(false)
    }
  }, [serverIP, serverPort, authKey, buildControlURL, api])

  return (
    <div className="mb-8 py-6 px-8 bg-white rounded-md shadow-2xl">
      <h3 className="text-2xl font-semibold mb-1">
        {needsAuth ? "重新连接控制服务器" : "连接到控制服务器"}
      </h3>
      <p className="text-gray-500 text-sm mb-6">
        {needsAuth
          ? "认证未完成或已失效，可以继续使用当前配置，也可以重新填写服务端。"
          : "请输入自定义控制服务器的连接信息。"}
      </p>

      {needsAuth && data.IPv4 !== "0" && data.IPv4 ? (
        <div className="mb-4 p-2 bg-gray-50 rounded text-sm text-gray-600">
          当前设备地址：{data.IPv4}
        </div>
      ) : null}

      <div className="mb-4">
        <label className="block text-sm font-medium text-gray-700 mb-1">
          服务器地址
        </label>
        <Input
          type="text"
          placeholder="例如 211.134.34.68 或 control.example.com"
          value={serverIP}
          onChange={(e) => setServerIP(e.target.value)}
          disabled={connecting}
        />
      </div>

      <div className="flex gap-3 mb-4">
        <div className="flex-1">
          <label className="block text-sm font-medium text-gray-700 mb-1">
            端口
          </label>
          <Input
            type="text"
            inputMode="numeric"
            placeholder={useHTTPS ? "443" : "80"}
            value={serverPort}
            onChange={(e) => setServerPort(e.target.value.replace(/\D/g, ""))}
            disabled={connecting}
          />
        </div>
        <div className="flex items-end pb-[0.35rem]">
          <label className="flex items-center gap-2 cursor-pointer select-none">
            <Toggle
              checked={useHTTPS}
              onChange={setUseHTTPS}
              disabled={connecting}
            />
            <span className="text-sm font-medium text-gray-700">HTTPS</span>
          </label>
        </div>
      </div>

      {serverIP.trim() && serverPort.trim() ? (
        <div className="mb-4 p-2 bg-gray-50 rounded text-sm text-gray-600 font-mono break-all">
          {buildControlURL()}
        </div>
      ) : null}

      <div className="mb-5">
        <label className="block text-sm font-medium text-gray-700 mb-1">
          认证密钥
          <span className="text-gray-400 font-normal ml-1">（可选）</span>
        </label>
        <Input
          type="text"
          placeholder="tskey-auth-..."
          value={authKey}
          onChange={(e) => setAuthKey(e.target.value)}
          disabled={connecting}
        />
        <p className="text-gray-400 text-xs mt-1">
          填写预认证密钥可直接连接；留空则在浏览器中完成认证。
        </p>
      </div>

      <Button
        onClick={handleConnect}
        className="w-full mb-2"
        intent="primary"
        loading={connecting}
        disabled={!serverIP.trim() || !serverPort.trim()}
      >
        {needsAuth ? "重新连接" : "连接"}
      </Button>

      {error ? <p className="text-red-500 text-sm mt-2">{error}</p> : null}
    </div>
  )
}
