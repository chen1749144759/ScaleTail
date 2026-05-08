// SPDX-License-Identifier: BSD-3-Clause

import React, { useState, useCallback } from "react"
import { useAPI } from "src/api"
import { NodeData } from "src/types"
import Button from "src/ui/button"
import Input from "src/ui/input"
import Toggle from "src/ui/toggle"

/**
 * ServerConfigView replaces the original LoginView.
 * It provides a GUI form for configuring the control server URL
 * (IP, port, HTTPS) in place of using the CLI `tailscale up --login-server=...`
 *
 * Rendered when the client is not connected to a tailnet:
 *   - "Stopped" / "NoState": show full server configuration form
 *   - "NeedsLogin": show re-auth or auth URL handling
 */
export default function ServerConfigView({ data }: { data: NodeData }) {
  const api = useAPI()

  // Form state
  const [serverIP, setServerIP] = useState("")
  const [serverPort, setServerPort] = useState("80")
  const [useHTTPS, setUseHTTPS] = useState(false)
  const [authKey, setAuthKey] = useState("")
  const [connecting, setConnecting] = useState(false)
  const [error, setError] = useState("")

  const needsConfig = data.Status === "Stopped" || data.Status === "NoState"
  const needsAuth = data.Status === "NeedsLogin"

  const buildControlURL = useCallback(() => {
    const scheme = useHTTPS ? "https" : "http"
    const port = serverPort.trim()
    // Omit default ports for cleaner URLs
    if (
      (useHTTPS && port === "443") ||
      (!useHTTPS && port === "80")
    ) {
      return `${scheme}://${serverIP.trim()}`
    }
    return `${scheme}://${serverIP.trim()}:${port}`
  }, [serverIP, serverPort, useHTTPS])

  const handleConnect = useCallback(async () => {
    const ip = serverIP.trim()
    if (!ip) {
      setError("请输入服务器 IP 地址")
      return
    }

    const controlURL = buildControlURL()
    setError("")
    setConnecting(true)

    try {
      await api({
        action: "up",
        data: {
          ControlURL: controlURL,
          AuthKey: authKey.trim() || undefined,
          Reauthenticate: !authKey.trim(), // with auth key: direct login; without: need browser auth
        },
      })
      // After api("up"), useSWR auto-refreshes /data.
      // If Status transitions to "Running", app.tsx switches to management view.
      // If an auth URL is returned, it's auto-opened in the browser.
    } catch (err) {
      setError(String(err))
    } finally {
      setConnecting(false)
    }
  }, [serverIP, serverPort, useHTTPS, authKey, buildControlURL, api])

  const handleReauth = useCallback(async () => {
    setError("")
    setConnecting(true)
    try {
      await api({
        action: "up",
        data: { Reauthenticate: true },
      })
    } catch (err) {
      setError(String(err))
    } finally {
      setConnecting(false)
    }
  }, [api])

  // Auth form (when Status=NeedsLogin — device needs re-authentication)
  if (needsAuth) {
    return (
      <div className="mb-8 py-6 px-8 bg-white rounded-md shadow-2xl">
        <h3 className="text-2xl font-semibold mb-3">
          {data.IPv4 !== "0" && data.IPv4 ? "重新认证" : "正在认证"}
        </h3>
        {data.IPv4 !== "0" && data.IPv4 ? (
          <>
            <p className="text-gray-700 mb-2">
              设备密钥已过期或需要重新认证。
            </p>
            <p className="text-gray-600 text-sm mb-4">
              当前设备地址：{data.IPv4}
            </p>
          </>
        ) : (
          <p className="text-gray-700 mb-4">
            正在连接控制服务器，请在浏览器中完成认证。
          </p>
        )}
        <Button
          onClick={handleReauth}
          className="w-full mb-3"
          intent="primary"
          loading={connecting}
        >
          重新认证
        </Button>
        {error && (
          <p className="text-red-500 text-sm mt-2">{error}</p>
        )}
      </div>
    )
  }

  // Server configuration form (Status=Stopped/NoState — never configured)
  return (
    <div className="mb-8 py-6 px-8 bg-white rounded-md shadow-2xl">
      <h3 className="text-2xl font-semibold mb-1">连接到控制服务器</h3>
      <p className="text-gray-500 text-sm mb-6">
        请输入自定义控制服务器的连接信息
      </p>

      {/* Server IP */}
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

      {/* Port + HTTPS row */}
      <div className="flex gap-3 mb-4">
        <div className="flex-1">
          <label className="block text-sm font-medium text-gray-700 mb-1">
            端口
          </label>
          <Input
            type="text"
            inputMode="numeric"
            placeholder="80"
            value={serverPort}
            onChange={(e) => {
              const v = e.target.value.replace(/\D/g, "")
              setServerPort(v)
            }}
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

      {/* Preview of URL */}
      {serverIP.trim() && (
        <div className="mb-4 p-2 bg-gray-50 rounded text-sm text-gray-600 font-mono break-all">
          {buildControlURL()}
        </div>
      )}

      {/* Auth Key */}
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
          填写预认证密钥可免浏览器认证，留空则在浏览器中完成认证
        </p>
      </div>

      {/* Connect button */}
      <Button
        onClick={handleConnect}
        className="w-full mb-2"
        intent="primary"
        loading={connecting}
        disabled={!serverIP.trim()}
      >
        连接
      </Button>

      {/* Error display */}
      {error && (
        <p className="text-red-500 text-sm mt-2">{error}</p>
      )}
    </div>
  )
}
