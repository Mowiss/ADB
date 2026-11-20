# Ressources d'Aldebaran

Fichiers chargés par l'appli à la demande (pas codés dans index.html).

- `mains/<race>.webp` : mains en vue 3D — humain (aussi pour elfe), troll, demon, trifide, droide.
- `persos/<race>-<h|f>.png` : aventuriers vus en pied par les autres joueurs (h = homme, f = femme). Race absente : pastille.
- `mobs/<clé>.png` : sprite 3D des MOBs dans la zone de jeu (fond transparent, ~300 px de haut pour une créature de taille humaine). La version HD du bestiaire est dans `../mobs/<clé>.webp`. Clé = nom du MOB en minuscules avec tirets (ex. `cyber-gobelin`), à ajouter dans `MOBSPR` (dungeon.js).
- `vehicules/facteur<n>.png` : véhicules vus en 3D selon leur facteur (pastille passagers/places ajoutée par l appli).
- `vege/<biome>-<arbre|buisson|herbe>.png` : végétation posée par le MJ (sprites croisés en X en 3D). Biomes : foret, marais, alien, desert, glace.
- `lum/<type>.png` : sources de lumière (cyber, lanterne, candelabre, bricole, brasero, moderne).
- `sprites/<style>.png` : contenants et dépouille en vue 3D — coffre, caisse, tech, jarre, viande.
- `themes/<thème>/` : textures du donjon 3D, thèmes `pierre`, `crypte`, `station`.
  Déposer `mur.png`, `sol.png`, `plafond.png`, `porte.png` (carrés, idéalement 64×64 ou 128×128).
  Un fichier absent garde la texture dessinée par l'appli.

Garder les noms en minuscules, sans accent.
