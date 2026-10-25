import CoreGraphics

enum MenuBarSection: Equatable {
    case shown
    case hidden
    /// Not drawn in any menu bar, typically switched off in System Settings › Menu Bar.
    case absent
}

/// Sorts expanded menu bar items around the separator and picks ⌘-drag drop points.
/// Frames are accessibility frames: global points with a top-left origin.
enum MenuBarLayout {
    static func section(of frame: CGRect, separator: CGRect, toggle: CGRect) -> MenuBarSection {
        // macOS 27 parks items it isn't drawing at a fixed spot off the bar's row, e.g. (-1, 970).
        guard abs(frame.midY - toggle.midY) <= toggle.height / 2 else { return .absent }
        // Neighbouring frames overlap by a point or two, so compare centres rather than edges.
        return frame.midX < separator.midX ? .hidden : .shown
    }

    /// Where a ⌘-drag has to end for the item to land in `section`. A lifted item lets the items
    /// left of it slide right, so both targets stay valid whichever side it starts on.
    static func dropPoint(for section: MenuBarSection, separator: CGRect, toggle: CGRect) -> CGPoint? {
        switch section {
        case .shown: return CGPoint(x: toggle.maxX + 4, y: toggle.midY)
        case .hidden: return dropPoint(leftOf: separator)
        case .absent: return nil
        }
    }

    /// Drop point that lands an item immediately left of the item at `frame`.
    static func dropPoint(leftOf frame: CGRect) -> CGPoint {
        CGPoint(x: frame.minX - 8, y: frame.midY)
    }

    // MARK: - Preferred positions

    /// Splits a macOS 27 layout key, "status:<owner>::<autosave name>", where the owner is the
    /// bundle identifier or, for some apps, the process name.
    static func parseKey(_ key: String) -> (owner: String, name: String)? {
        guard key.hasPrefix("status:"), let range = key.range(of: "::") else { return nil }
        let owner = String(key[key.index(key.startIndex, offsetBy: 7)..<range.lowerBound])
        return owner.isEmpty ? nil : (owner, String(key[range.upperBound...]))
    }

    /// Preferred positions count from the right edge, so a larger value sits further left.
    /// Everything left of the rightmost block item (separator or spacer) is pushed out when collapsed.
    static func section(position: Double, blockEdge: Double) -> MenuBarSection {
        position > blockEdge ? .hidden : .shown
    }

    /// A position immediately left of `anchor`, halfway to the next item so nothing else is crossed.
    static func position(leftOf anchor: Double, existing: [Double]) -> Double {
        let next = existing.filter { $0 > anchor }.min()
        return next.map { (anchor + $0) / 2 } ?? anchor + 16
    }

    /// A position immediately right of `anchor`; the clock sits at 0, so it never goes below that.
    static func position(rightOf anchor: Double, existing: [Double]) -> Double {
        let next = existing.filter { $0 < anchor }.max() ?? 0
        return (anchor + next) / 2
    }
}
