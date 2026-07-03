package translate

import (
	"strings"
)

type ServiceTranslation struct {
	PL string
	DE string
	EN string
	NL string
	DK string
	FR string
	IT string
}

var dictionary = []ServiceTranslation{
	{
		PL: "Remont łazienki",
		DE: "Badsanierung",
		EN: "Bathroom renovation",
		NL: "Badkamerrenovatie",
		DK: "Badeværelsesrenovering",
		FR: "Rénovation de salle de bain",
		IT: "Ristrutturazione bagno",
	},
	{
		PL: "Renowacja łazienki",
		DE: "Badrenovierung",
		EN: "Bathroom renovation",
		NL: "Badkamerrenovatie",
		DK: "Renovering af badeværelse",
		FR: "Rénovation de salle de bain",
		IT: "Ristrutturazione bagno",
	},
	{
		PL: "Prace wyburzeniowe",
		DE: "Abbrucharbeiten",
		EN: "Demolition works",
		NL: "Sloopwerkzaamheden",
		DK: "Nedrivningsarbejde",
		FR: "Travaux de démolition",
		IT: "Lavori di demolizione",
	},
	{
		PL: "Adaptacja poddasza",
		DE: "Dachbodenausbau",
		EN: "Loft conversion",
		NL: "Zolderverbouwing",
		DK: "Loftsudnyttelse",
		FR: "Aménagement des combles",
		IT: "Ristrutturazione sottotetto",
	},
	{
		PL: "Drzwi przesuwne",
		DE: "Schiebetüren",
		EN: "Sliding doors",
		NL: "Schuifpuien",
		DK: "Skydedøre",
		FR: "Portes coulissantes",
		IT: "Porte scorrevoli",
	},
	{
		PL: "Wylewka samopoziomująca",
		DE: "Ausgleichsmasse",
		EN: "Self-leveling compound",
		NL: "Egaline",
		DK: "Selvnivellerende spartelmasse",
		FR: "Ragréage autonivelant",
		IT: "Livellina autolivellante",
	},
	{
		PL: "Rozbudowa domu",
		DE: "Anbau",
		EN: "House extension",
		NL: "Aanbouw",
		DK: "Tilbygning",
		FR: "Extension de maison",
		IT: "Ampliamento casa",
	},
	{
		PL: "Kierownictwo budowy",
		DE: "Bauleitung",
		EN: "Construction management",
		NL: "Bouwbegeleiding",
		DK: "Byggeledelse",
		FR: "Direction de chantier",
		IT: "Direzione lavori",
	},
	{
		PL: "Wycinka drzew",
		DE: "Baumfällung",
		EN: "Tree felling",
		NL: "Bomen kappen",
		DK: "Træfældning",
		FR: "Abattage d'arbres",
		IT: "Abbattimento alberi",
	},
	{
		PL: "Sadzenie roślin",
		DE: "Bepflanzung",
		EN: "Planting",
		NL: "Beplanting",
		DK: "Plantning af planter",
		FR: "Plantation",
		IT: "Piantumazione",
	},
	{
		PL: "Układanie podłóg",
		DE: "Bodenverlegung",
		EN: "Floor laying",
		NL: "Vloer leggen",
		DK: "Gulvlægning",
		FR: "Pose de sol",
		IT: "Posa pavimenti",
	},
	{
		PL: "Sufity podwieszane",
		DE: "Decken abhängen",
		EN: "Suspended ceilings",
		NL: "Verlaagde plafonds",
		DK: "Nedhængte lofter",
		FR: "Plafonds suspendus",
		IT: "Controsoffitti",
	},
	{
		PL: "Ocieplanie elewacji",
		DE: "Fassadendämmung",
		EN: "Facade insulation",
		NL: "Gevelisolatie",
		DK: "Facadeisolering",
		FR: "Isolation de façade",
		IT: "Isolamento facciata",
	},
	{
		PL: "Układanie płytek",
		DE: "Fliesenlegen",
		EN: "Tiling",
		NL: "Betegelen",
		DK: "Fliselægning",
		FR: "Pose de carrelage",
		IT: "Posa piastrelle",
	},
	{
		PL: "Demontaż wnętrz",
		DE: "Entkernung",
		EN: "Interior strip-out",
		NL: "Strippen van interieur",
		DK: "Rydning af indretning",
		FR: "Curage de building",
		IT: "Sventramento interni",
	},
	{
		PL: "Posadzka epoksydowa",
		DE: "Epoxidharz Boden",
		EN: "Epoxy flooring",
		NL: "Epoxyvloer",
		DK: "Epoxygulv",
		FR: "Revêtement de sol époxy",
		IT: "Pavimentazione epossidica",
	},
	{
		PL: "Wylewka",
		DE: "Estrich",
		EN: "Screed",
		NL: "Dekvloer",
		DK: "Gulvafretning",
		FR: "Chape",
		IT: "Massetto",
	},
	{
		PL: "Montaż kuchni",
		DE: "Küchenmontage",
		EN: "Kitchen fitting",
		NL: "Keukenmontage",
		DK: "Køkkenmontering",
		FR: "Pose de cuisine",
		IT: "Montaggio cucina",
	},
	{
		PL: "Prace szpachlowe",
		DE: "Spachtelarbeiten",
		EN: "Plastering",
		NL: "Stucwerk",
		DK: "Spartelarbejde",
		FR: "Travaux de plâtrerie",
		IT: "Rasatura pareti",
	},
	{
		PL: "Sucha zabudowa",
		DE: "Trockenbau",
		EN: "Drywall installation",
		NL: "Droogbouw",
		DK: "Gipsplademontage",
		FR: "Cloison sèche",
		IT: "Cartongesso",
	},
	{
		PL: "Lukarna",
		DE: "Dachgaube",
		EN: "Dormer window",
		NL: "Dakkapel",
		DK: "Tagkvist",
		FR: "Lucarne",
		IT: "Lucernario",
	},
	{
		PL: "Balustrady",
		DE: "Geländer",
		EN: "Balustrades",
		NL: "Balustrades",
		DK: "Gelændere",
		FR: "Garde-corps",
		IT: "Ringhiere",
	},
	{
		PL: "Generalny wykonawca",
		DE: "Generalunternehmer",
		EN: "General contractor",
		NL: "Hoofdaannemer",
		DK: "Hovedentreprenør",
		FR: "Entreprise générale",
		IT: "Impresa generale",
	},
	{
		PL: "Montaż mebli",
		DE: "Möbelmontage",
		EN: "Furniture assembly",
		NL: "Meubelmontage",
		DK: "Møbelmontering",
		FR: "Montage de meubles",
		IT: "Montaggio mobili",
	},
	{
		PL: "Układanie trawnika",
		DE: "Rollrasen verlegen",
		EN: "Turf laying",
		NL: "Graszoden leggen",
		DK: "Rullegræs",
		FR: "Pose de gazon en rouleau",
		IT: "Posa prato a rotoli",
	},
	{
		PL: "Instalacje sanitarne",
		DE: "Sanitärinstallation",
		EN: "Sanitary installations",
		NL: "Sanitairtechniek",
		DK: "Sanitære installationer",
		FR: "Installations sanitaires",
		IT: "Impianti sanitari",
	},
	{
		PL: "Termoizolacja",
		DE: "Wärmedämmung",
		EN: "Thermal insulation",
		NL: "Thermische isolatie",
		DK: "Varmeisolering",
		FR: "Isolation thermique",
		IT: "Isolamento termico",
	},
	{
		PL: "Instalacje CO",
		DE: "Heizungsinstallation",
		EN: "Central heating installation",
		NL: "Verwarmingsinstallatie",
		DK: "Varmeinstallationer",
		FR: "Installation de chauffage central",
		IT: "Impianto di riscaldamento",
	},
	{
		PL: "Instalacje hydrauliczne",
		DE: "Sanitär und Heizung",
		EN: "Plumbing services",
		NL: "Loodgieterswerk",
		DK: "VVS-arbejde",
		FR: "Services de plomberie",
		IT: "Servizi idraulici",
	},
	{
		PL: "Instalacje elektryczne",
		DE: "Elektroinstallation",
		EN: "Electrical installations",
		NL: "Elektrotechnische installatie",
		DK: "Elinstallationer",
		FR: "Installations électriques",
		IT: "Impianti elettrici",
	},
	{
		PL: "Klimatyzacja",
		DE: "Klimaanlage",
		EN: "Air conditioning",
		NL: "Airconditioning",
		DK: "Klimaanlæg",
		FR: "Climatisation",
		IT: "Aria condizionata",
	},
	{
		PL: "Ogrzewanie podłogowe",
		DE: "Fußbodenheizung",
		EN: "Underfloor heating",
		NL: "Vloerverwarming",
		DK: "Gulvvarme",
		FR: "Plancher chauffant",
		IT: "Riscaldamento a pavimento",
	},
	{
		PL: "Systemy alarmowe",
		DE: "Alarmanlagen",
		EN: "Alarm systems",
		NL: "Alarmsystemen",
		DK: "Alarmsystemer",
		FR: "Systèmes d'alarme",
		IT: "Sistemi di allarme",
	},
	{
		PL: "Kostka brukowa",
		DE: "Pflasterarbeiten",
		EN: "Paving",
		NL: "Bestrating",
		DK: "Flisebelægning",
		FR: "Pavage",
		IT: "Pavimentazione externa",
	},
	{
		PL: "Ogrodzenia murowane",
		DE: "Mauereinzäunung",
		EN: "Brick fencing",
		NL: "Gemetselde schutting",
		DK: "Muret hegn",
		FR: "Clôture maçonnée",
		IT: "Recinzione in muratura",
	},
	{
		PL: "Ogrodzenia panelowe",
		DE: "Stabmattenzaun",
		EN: "Panel fencing",
		NL: "Paneelhekwerk",
		DK: "Panelhegn",
		FR: "Clôture en panneaux",
		IT: "Recinzione a pannelli",
	},
	{
		PL: "Okna PCV",
		DE: "Kunststofffenster",
		EN: "uPVC Windows",
		NL: "Kunststof kozijnen",
		DK: "Plastikvinduer",
		FR: "Fenêtres en PVC",
		IT: "Finestre in PVC",
	},
	{
		PL: "Okna drewniane",
		DE: "Holzfenster",
		EN: "Timber Windows",
		NL: "Houten kozijnen",
		DK: "Trævinduer",
		FR: "Fenêtres en bois",
		IT: "Finestre in legno",
	},
	{
		PL: "Naprawa dachu",
		DE: "Dachreparatur",
		EN: "Roof repair",
		NL: "Dahreparatie",
		DK: "Tagreparation",
		FR: "Réparation de toiture",
		IT: "Riparazione tetto",
	},
	{
		PL: "Montaż dachu",
		DE: "Dacheindeckung",
		EN: "Roof installation",
		NL: "Dakdekken",
		DK: "Tagdækning",
		FR: "Installation de toiture",
		IT: "Copertura tetto",
	},
	{
		PL: "Malowanie elewacji",
		DE: "Fassadenanstrich",
		EN: "Facade painting",
		NL: "Gevel schilderen",
		DK: "Facademaling",
		FR: "Peinture de façade",
		IT: "Pintura facciata",
	},
}

var lookup map[string]ServiceTranslation

func init() {
	lookup = make(map[string]ServiceTranslation)
	for _, t := range dictionary {
		if t.PL != "" {
			lookup[strings.ToLower(t.PL)] = t
		}
		if t.DE != "" {
			lookup[strings.ToLower(t.DE)] = t
		}
		if t.EN != "" {
			lookup[strings.ToLower(t.EN)] = t
		}
		if t.NL != "" {
			lookup[strings.ToLower(t.NL)] = t
		}
		if t.DK != "" {
			lookup[strings.ToLower(t.DK)] = t
		}
		if t.FR != "" {
			lookup[strings.ToLower(t.FR)] = t
		}
		if t.IT != "" {
			lookup[strings.ToLower(t.IT)] = t
		}
	}
}

// DictionaryTranslate looks up the term in our local industry dictionary.
// targetLang is one of "pl", "de", "en", "nl", "dk", "fr", "it" (case insensitive).
// Returns the translated string if found, and true. Otherwise returns "", false.
func DictionaryTranslate(term string, targetLang string) (string, bool) {
	termClean := strings.TrimSpace(strings.ToLower(term))
	t, ok := lookup[termClean]
	if !ok {
		return "", false
	}

	switch strings.ToLower(targetLang) {
	case "pl":
		return t.PL, t.PL != ""
	case "de":
		return t.DE, t.DE != ""
	case "en":
		return t.EN, t.EN != ""
	case "nl":
		return t.NL, t.NL != ""
	case "dk":
		return t.DK, t.DK != ""
	case "fr":
		return t.FR, t.FR != ""
	case "it":
		return t.IT, t.IT != ""
	default:
		return "", false
	}
}
