// Pure helpers behind the `artist` command, free of MusicKit so they can be
// unit tested.
import Foundation

/// Turns catalog editorial notes, which are HTML fragments, into plain text
/// for the terminal.
public enum EditorialText {
    /// Line breaks and paragraph ends become newlines, other tags are
    /// dropped, entities are decoded (unknown ones are kept as written),
    /// control characters other than newlines are removed (control
    /// whitespace such as a tab counts as a space), and runs of whitespace
    /// collapse to single spaces with blank lines removed.
    public static func plain(_ html: String) -> String {
        var text = html.replacing(#/(?i)<br\s*/?>|</p\s*>|</div\s*>/#, with: "\n")
        text = text.replacing(#/</?[A-Za-z!][^>]*>/#, with: "")
        text = text.replacing(#/&(#[0-9]+|#[xX][0-9A-Fa-f]+|[A-Za-z]+);/#) { match in
            decode(String(match.output.1)) ?? String(match.output.0)
        }
        return withoutControls(text)
            .split(separator: "\n", omittingEmptySubsequences: false)
            .map { $0.split(whereSeparator: \.isWhitespace).joined(separator: " ") }
            .filter { !$0.isEmpty }
            .joined(separator: "\n")
    }

    /// Removes C0/C1 control characters, which (like a decoded "&#27;",
    /// ESC) would reach the terminal as control sequences. Newlines stay;
    /// control whitespace (tab, carriage return, NEL) becomes a space.
    private static func withoutControls(_ text: String) -> String {
        var kept = String.UnicodeScalarView()
        for scalar in text.unicodeScalars {
            if scalar == "\n" || scalar.properties.generalCategory != .control {
                kept.append(scalar)
            } else if scalar.properties.isWhitespace {
                kept.append(" ")
            }
        }
        return String(kept)
    }

    private static let named: [String: String] = [
        "amp": "&", "lt": "<", "gt": ">", "quot": "\"", "apos": "'", "nbsp": " ",
        "mdash": "—", "ndash": "–", "hellip": "…", "rsquo": "’", "lsquo": "‘",
        "rdquo": "”", "ldquo": "“",
    ]

    /// Decodes one entity body ("amp", "#39", "#x2014"); nil when unknown.
    private static func decode(_ entity: String) -> String? {
        guard entity.hasPrefix("#") else { return named[entity] }
        let digits = entity.dropFirst()
        let value = digits.first == "x" || digits.first == "X"
            ? UInt32(digits.dropFirst(), radix: 16)
            : UInt32(digits, radix: 10)
        return value.flatMap(Unicode.Scalar.init).map { String(Character($0)) }
    }
}

/// Where an artist is from and when it was born or formed, read from the
/// Apple Music API's extended artist attributes (`extend=origin,bornOrFormed`).
/// Neither attribute is part of MusicKit's public model, so both are best
/// effort: anything missing or malformed is an empty string.
public struct ArtistFacts: Equatable {
    public let origin: String
    public let formed: String

    public init(origin: String, formed: String) {
        self.origin = origin
        self.formed = formed
    }

    /// Parses `data[0].attributes.origin` and `.bornOrFormed` from a catalog
    /// artist response. A full date ("1993-01-01") is shortened to its year.
    public static func parse(_ data: Data) -> ArtistFacts {
        guard let object = try? JSONSerialization.jsonObject(with: data) as? [String: Any],
              let items = object["data"] as? [[String: Any]],
              let attributes = items.first?["attributes"] as? [String: Any]
        else { return ArtistFacts(origin: "", formed: "") }
        var formed = attributes["bornOrFormed"] as? String ?? ""
        if formed.wholeMatch(of: #/\d{4}-\d{2}-\d{2}/#) != nil {
            formed = String(formed.prefix(4))
        }
        return ArtistFacts(origin: attributes["origin"] as? String ?? "", formed: formed)
    }
}
