import AppKit

/// Draws the level-accurate battery icons; symbol icons come straight from SF Symbols.
enum MenuBarIconRenderer {
    static func image(for icon: MenuBarIcon) -> NSImage? {
        switch icon {
        case .symbol(let name):
            let image = NSImage(systemSymbolName: name, accessibilityDescription: nil)
            image?.isTemplate = true
            return image
        case .battery(let style, let percent, let charging):
            switch style {
            case .macOS: return battery(percent: percent, charging: charging, colored: false)
            case .macOSColor: return battery(percent: percent, charging: charging, colored: true)
            case .iOS: return iOSBattery(percent: percent, charging: charging)
            case .ring: return ring(percent: percent, charging: charging)
            case .hidden, .battery, .appIcon: return nil
            }
        }
    }

    private static let bodySize = NSSize(width: 22, height: 11)
    private static let capWidth: CGFloat = 1.6
    private static var canvas: NSSize { NSSize(width: bodySize.width + capWidth + 1, height: 13) }

    private static var bodyRect: NSRect {
        NSRect(x: 0.5, y: (canvas.height - bodySize.height) / 2, width: bodySize.width, height: bodySize.height)
    }

    private static func capPath() -> NSBezierPath {
        let rect = NSRect(x: bodyRect.maxX + 0.8, y: bodyRect.midY - 2, width: capWidth, height: 4)
        return NSBezierPath(roundedRect: rect, xRadius: 0.8, yRadius: 0.8)
    }

    private static func fillColor(percent: Int, charging: Bool) -> NSColor {
        if charging { return .systemGreen }
        return percent <= 20 ? .systemRed : .systemGreen
    }

    /// Outline plus a fill clipped to the level, like the macOS menu bar battery.
    private static func battery(percent: Int, charging: Bool, colored: Bool) -> NSImage {
        let image = NSImage(size: canvas, flipped: false) { _ in
            let outline = colored ? NSColor.labelColor : NSColor.black
            outline.withAlphaComponent(colored ? 0.55 : 0.5).setStroke()
            let body = NSBezierPath(roundedRect: bodyRect.insetBy(dx: 0.5, dy: 0.5), xRadius: 3, yRadius: 3)
            body.lineWidth = 1
            body.stroke()
            outline.withAlphaComponent(colored ? 0.55 : 0.5).setFill()
            capPath().fill()

            let inner = bodyRect.insetBy(dx: 2, dy: 2)
            let fill = NSRect(x: inner.minX, y: inner.minY, width: max(1.5, inner.width * CGFloat(percent) / 100), height: inner.height)
            (colored ? fillColor(percent: percent, charging: charging) : NSColor.black).setFill()
            NSBezierPath(roundedRect: fill, xRadius: 1.5, yRadius: 1.5).fill()

            if charging {
                drawBolt(in: bodyRect, knockout: !colored, color: colored ? .labelColor : .black)
            }
            return true
        }
        image.isTemplate = !colored
        return image
    }

    /// Solid body with the percentage knocked out of the filled part, like iOS.
    private static func iOSBattery(percent: Int, charging: Bool) -> NSImage {
        let image = NSImage(size: canvas, flipped: false) { _ in
            let body = NSBezierPath(roundedRect: bodyRect, xRadius: 3.5, yRadius: 3.5)
            NSColor.black.withAlphaComponent(0.35).setFill()
            body.fill()
            capPath().fill()

            NSGraphicsContext.saveGraphicsState()
            body.addClip()
            NSColor.black.setFill()
            NSRect(x: bodyRect.minX, y: bodyRect.minY, width: bodyRect.width * CGFloat(percent) / 100, height: bodyRect.height).fill()
            NSGraphicsContext.restoreGraphicsState()

            let text = "\(percent)"
            let font = NSFont.systemFont(ofSize: percent == 100 ? 8 : 9, weight: .bold)
            let attributes: [NSAttributedString.Key: Any] = [.font: font, .foregroundColor: NSColor.black]
            let size = (text as NSString).size(withAttributes: attributes)
            let origin = NSPoint(x: bodyRect.midX - size.width / 2 - (charging ? 2 : 0), y: bodyRect.midY - size.height / 2)
            NSGraphicsContext.current?.compositingOperation = .destinationOut
            (text as NSString).draw(at: origin, withAttributes: attributes)
            if charging {
                let boltRect = NSRect(x: origin.x + size.width + 0.5, y: bodyRect.minY + 1.5, width: 5, height: bodyRect.height - 3)
                boltPath(in: boltRect).fill()
            }
            NSGraphicsContext.current?.compositingOperation = .sourceOver
            return true
        }
        image.isTemplate = true
        return image
    }

    private static func ring(percent: Int, charging: Bool) -> NSImage {
        let size = NSSize(width: 14, height: 14)
        let image = NSImage(size: size, flipped: false) { _ in
            let center = NSPoint(x: size.width / 2, y: size.height / 2)
            let track = NSBezierPath()
            track.appendArc(withCenter: center, radius: 5.5, startAngle: 0, endAngle: 360)
            track.lineWidth = 2.2
            NSColor.black.withAlphaComponent(0.25).setStroke()
            track.stroke()

            let arc = NSBezierPath()
            arc.appendArc(withCenter: center, radius: 5.5, startAngle: 90, endAngle: 90 - 360 * CGFloat(percent) / 100, clockwise: true)
            arc.lineWidth = 2.2
            arc.lineCapStyle = .round
            NSColor.black.setStroke()
            arc.stroke()

            if charging {
                NSColor.black.setFill()
                boltPath(in: NSRect(x: center.x - 2, y: center.y - 3, width: 4, height: 6)).fill()
            }
            return true
        }
        image.isTemplate = true
        return image
    }

    /// Template icons punch the bolt out of the fill so it reads on any menu bar; coloured icons draw it.
    private static func drawBolt(in rect: NSRect, knockout: Bool, color: NSColor) {
        let bolt = boltPath(in: NSRect(x: rect.midX - 3.5, y: rect.minY + 1, width: 7, height: rect.height - 2))
        if knockout {
            NSGraphicsContext.current?.compositingOperation = .destinationOut
            bolt.lineWidth = 1.3
            bolt.stroke()
            NSGraphicsContext.current?.compositingOperation = .sourceOver
            NSColor.black.setFill()
            bolt.fill()
        } else {
            NSColor.windowBackgroundColor.setStroke()
            bolt.lineWidth = 1.2
            bolt.stroke()
            color.setFill()
            bolt.fill()
        }
    }

    private static func boltPath(in rect: NSRect) -> NSBezierPath {
        let path = NSBezierPath()
        path.move(to: NSPoint(x: rect.minX + rect.width * 0.62, y: rect.maxY))
        path.line(to: NSPoint(x: rect.minX, y: rect.minY + rect.height * 0.42))
        path.line(to: NSPoint(x: rect.minX + rect.width * 0.48, y: rect.minY + rect.height * 0.42))
        path.line(to: NSPoint(x: rect.minX + rect.width * 0.38, y: rect.minY))
        path.line(to: NSPoint(x: rect.maxX, y: rect.minY + rect.height * 0.58))
        path.line(to: NSPoint(x: rect.minX + rect.width * 0.52, y: rect.minY + rect.height * 0.58))
        path.close()
        return path
    }
}
