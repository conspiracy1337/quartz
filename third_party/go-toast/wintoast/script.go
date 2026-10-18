package wintoast

import "encoding/base64"

// powershellScript is the fallback script that shows a toast when the
// Windows Runtime COM API is unavailable. Upstream filled a text/template
// with the XML inside a double-quoted here-string, where PowerShell expands
// $(...) from the notification's text; here both values travel as base64
// and are decoded by the script, so no field can become code.
func powershellScript(appID, xml string) string {
	if appID == "" {
		appID = "Windows App"
	}
	return `[Windows.UI.Notifications.ToastNotificationManager, Windows.UI.Notifications, ContentType = WindowsRuntime] | Out-Null
[Windows.UI.Notifications.ToastNotification, Windows.UI.Notifications, ContentType = WindowsRuntime] | Out-Null
[Windows.Data.Xml.Dom.XmlDocument, Windows.Data.Xml.Dom.XmlDocument, ContentType = WindowsRuntime] | Out-Null

$APP_ID = ` + psDecode(appID) + `

$template = ` + psDecode(xml) + `

$xml = New-Object Windows.Data.Xml.Dom.XmlDocument
$xml.LoadXml($template)
$toast = New-Object Windows.UI.Notifications.ToastNotification $xml
[Windows.UI.Notifications.ToastNotificationManager]::CreateToastNotifier($APP_ID).Show($toast)`
}

// psDecode is a PowerShell expression that evaluates to s.
func psDecode(s string) string {
	return "[System.Text.Encoding]::UTF8.GetString([System.Convert]::FromBase64String('" + base64.StdEncoding.EncodeToString([]byte(s)) + "'))"
}
