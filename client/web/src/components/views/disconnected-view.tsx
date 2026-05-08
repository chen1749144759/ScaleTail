// SPDX-License-Identifier: BSD-3-Clause

import React from "react"
import { Link } from "wouter"
import Button from "src/ui/button"

/**
 * DisconnectedView is rendered after node logout.
 * In our custom build, it offers a way back to the configuration view.
 */
export default function DisconnectedView() {
  return (
    <div className="mt-8 py-6 px-8 bg-white rounded-md shadow-2xl text-center">
      <p className="mt-6 text-gray-700 mb-4">
        已从此设备注销。需要重新配置控制服务器才能重新连接。
      </p>
      <Link to="/">
        <Button intent="primary">返回配置</Button>
      </Link>
    </div>
  )
}
